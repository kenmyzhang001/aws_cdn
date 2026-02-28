package services

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/aws"
	"fmt"
	"time"

	"gorm.io/gorm"
)

var RegionToAMI = map[string]string{
	"ap-east-1": "ami-0ee9ac79045aa1d6b",
}

var RegionToSecurityGroup = map[string]string{
	"ap-east-1": "sg-0b6fa9e47ec4886dd",
}

const DefaultInstanceType = "t3.nano"

type RegionConfig struct {
	Region          string `json:"region"`
	AMIID           string `json:"ami_id"`
	SecurityGroupID string `json:"security_group_id"`
}

type Ec2InstanceService struct {
	db  *gorm.DB
	cfg *config.AWSConfig
}

func NewEc2InstanceService(db *gorm.DB, cfg *config.AWSConfig) *Ec2InstanceService {
	return &Ec2InstanceService{db: db, cfg: cfg}
}

func (s *Ec2InstanceService) GetRegionConfig() []RegionConfig {
	var out []RegionConfig
	for region, amiID := range RegionToAMI {
		sgID := RegionToSecurityGroup[region]
		out = append(out, RegionConfig{Region: region, AMIID: amiID, SecurityGroupID: sgID})
	}
	return out
}

type CreateRequest struct {
	Region string `json:"region" binding:"required"`
	Name   string `json:"name" binding:"required"`
	Note   string `json:"note"`
}

type UpdateRequest struct {
	Name string `json:"name"`
	Note string `json:"note"`
}

func (s *Ec2InstanceService) Create(req *CreateRequest) (*models.Ec2Instance, error) {
	amiID, ok := RegionToAMI[req.Region]
	if !ok {
		return nil, fmt.Errorf("不支持的地区: %s", req.Region)
	}
	sgID := RegionToSecurityGroup[req.Region]
	if sgID == "" {
		return nil, fmt.Errorf("地区 %s 未配置安全组", req.Region)
	}

	client, err := aws.NewEC2Client(s.cfg, req.Region)
	if err != nil {
		return nil, fmt.Errorf("创建 EC2 客户端失败: %w", err)
	}

	instanceID, err := aws.RunInstance(client, amiID, DefaultInstanceType, sgID, req.Name)
	if err != nil {
		return nil, fmt.Errorf("启动实例失败: %w", err)
	}

	inst := &models.Ec2Instance{
		Name:            req.Name,
		Region:          req.Region,
		AMIID:           amiID,
		SecurityGroupID: sgID,
		InstanceType:    DefaultInstanceType,
		AWSInstanceID:   instanceID,
		State:           "pending",
		Note:            req.Note,
	}
	if err := s.db.Create(inst).Error; err != nil {
		return nil, fmt.Errorf("保存实例记录失败: %w", err)
	}
	return inst, nil
}

// List 查数据库并填充公网 IP（从 AWS DescribeInstances 获取），返回未软删除的实例
func (s *Ec2InstanceService) List(page, pageSize int, region string) ([]*models.Ec2Instance, int64, error) {
	var list []*models.Ec2Instance
	q := s.db.Model(&models.Ec2Instance{})
	if region != "" {
		q = q.Where("region = ?", region)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	if err := q.Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	// 按 region 分组，批量查 AWS 公网 IP 并填充
	byRegion := make(map[string][]*models.Ec2Instance)
	for _, inst := range list {
		if inst.AWSInstanceID != "" {
			byRegion[inst.Region] = append(byRegion[inst.Region], inst)
		}
	}
	for r, insts := range byRegion {
		client, err := aws.NewEC2Client(s.cfg, r)
		if err != nil {
			continue
		}
		ids := make([]string, 0, len(insts))
		for _, i := range insts {
			ids = append(ids, i.AWSInstanceID)
		}
		ipMap, err := aws.GetInstancesPublicIPs(client, ids)
		if err != nil {
			continue
		}
		for _, i := range insts {
			if ip, ok := ipMap[i.AWSInstanceID]; ok {
				i.PublicIP = ip
				// 持久化到数据库，便于回收站等场景显示
				_ = s.db.Model(i).Update("public_ip", ip).Error
			}
		}
	}
	return list, total, nil
}

func (s *Ec2InstanceService) GetByID(id uint) (*models.Ec2Instance, error) {
	var inst models.Ec2Instance
	if err := s.db.First(&inst, id).Error; err != nil {
		return nil, err
	}
	return &inst, nil
}

func (s *Ec2InstanceService) Update(id uint, req *UpdateRequest) (*models.Ec2Instance, error) {
	var inst models.Ec2Instance
	if err := s.db.First(&inst, id).Error; err != nil {
		return nil, err
	}
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	updates["note"] = req.Note
	if err := s.db.Model(&inst).Updates(updates).Error; err != nil {
		return nil, err
	}
	s.db.First(&inst, id)
	return &inst, nil
}

func (s *Ec2InstanceService) Delete(id uint) error {
	var inst models.Ec2Instance
	if err := s.db.First(&inst, id).Error; err != nil {
		return err
	}
	if inst.AWSInstanceID != "" {
		client, err := aws.NewEC2Client(s.cfg, inst.Region)
		if err != nil {
			logger.GetLogger().WithError(err).Warn("创建 EC2 客户端失败，继续软删除记录")
		} else {
			if err := aws.TerminateInstance(client, inst.AWSInstanceID); err != nil {
				logger.GetLogger().WithError(err).WithField("instance_id", inst.AWSInstanceID).Warn("终止实例失败，继续软删除记录")
			}
		}
	}
	lifetimeHours := time.Since(inst.CreatedAt).Hours()
	if err := s.db.Model(&inst).Updates(map[string]interface{}{
		"state":          "terminated",
		"lifetime_hours": lifetimeHours,
	}).Error; err != nil {
		return fmt.Errorf("更新实例状态失败: %w", err)
	}
	if err := s.db.Delete(&inst).Error; err != nil {
		return fmt.Errorf("软删除失败: %w", err)
	}
	return nil
}

func (s *Ec2InstanceService) ListDeleted(page, pageSize int) ([]*models.Ec2Instance, int64, error) {
	var list []*models.Ec2Instance
	q := s.db.Unscoped().Where("deleted_at IS NOT NULL").Model(&models.Ec2Instance{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	if err := q.Order("deleted_at DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}
