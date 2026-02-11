package models

// SiteDailyData 站点日数据结构（用于缓存/API）
type SiteDailyData struct {
	SiteName string         `json:"siteName"`
	Stats    []DailyStats   `json:"stats"`
	Summary  *DailySummary  `json:"summary"`
}

// DailyStats 渠道日统计
type DailyStats struct {
	ChannelCode    string  `json:"channelCode"`
	ChannelName    string  `json:"channelName"`
	RegCount       int     `json:"regCount"`       // 注册数
	FirstPayCount  int     `json:"firstPayCount"`  // 首充数
	PayAmount      float64 `json:"payAmount"`      // 充值金额
	WithdrawAmount float64 `json:"withdrawAmount"` // 提款金额
	NetAmount      float64 `json:"netAmount"`      // 充提差
	Date           string  `json:"date"`
	ChargeUserCount int   `json:"chargeUserCount"` // 充值人数
	CashUserCount   int   `json:"cashUserCount"`   // 提现人数
}

// DailySummary 站点日汇总
type DailySummary struct {
	SiteName      string       `json:"siteName"`
	Date          string       `json:"date"`
	ChannelStats  []DailyStats `json:"channelStats"`
	TotalReg      int          `json:"totalReg"`
	TotalFirstPay int          `json:"totalFirstPay"`
	TotalPay      float64      `json:"totalPay"`
	TotalWithdraw float64      `json:"totalWithdraw"`
	TotalNet      float64      `json:"totalNet"` // 总充提差
}
