import request from '@/api/request'

/**
 * 优化的文件上传函数，支持进度显示和更好的性能
 * @param {string} url - 上传接口地址
 * @param {FormData} formData - 表单数据
 * @param {Object} options - 配置选项
 * @param {Function} onProgress - 进度回调函数 (progress: number) => void
 * @returns {Promise} 上传结果
 */
export function uploadFile(url, formData, options = {}, onProgress) {
  return new Promise((resolve, reject) => {
    const config = {
      headers: {
        'Content-Type': 'multipart/form-data',
        ...options.headers,
      },
      timeout: options.timeout || 600000, // 默认10分钟
      onUploadProgress: (progressEvent) => {
        if (onProgress && progressEvent.total) {
          const percentCompleted = Math.round(
            (progressEvent.loaded * 100) / progressEvent.total
          )
          onProgress(percentCompleted)
        } else if (onProgress && progressEvent.loaded) {
          // 如果无法获取总大小，至少显示已上传的字节数
          onProgress(-1) // 使用 -1 表示不确定进度
        }
      },
      // 优化上传性能
      maxContentLength: Infinity,
      maxBodyLength: Infinity,
      // 禁用请求转换，直接发送 FormData
      transformRequest: [(data) => data],
    }

    request
      .post(url, formData, config)
      .then((response) => {
        if (onProgress) {
          onProgress(100) // 确保进度显示为100%
        }
        resolve(response)
      })
      .catch((error) => {
        reject(error)
      })
  })
}

/**
 * 分片上传（如果后端支持）
 * 当前后端不支持分片，但可以用于显示更准确的进度
 * @param {string} url - 上传接口地址
 * @param {File} file - 要上传的文件
 * @param {Object} additionalData - 额外的表单数据
 * @param {Function} onProgress - 进度回调函数
 * @param {number} chunkSize - 分片大小（字节），默认 5MB
 * @returns {Promise} 上传结果
 */
export async function uploadFileInChunks(
  url,
  file,
  additionalData = {},
  onProgress,
  chunkSize = 5 * 1024 * 1024 // 5MB
) {
  // 如果文件小于分片大小，直接上传
  if (file.size <= chunkSize) {
    const formData = new FormData()
    Object.keys(additionalData).forEach((key) => {
      formData.append(key, additionalData[key])
    })
    formData.append('file', file)

    return uploadFile(url, formData, {}, onProgress)
  }

  // 大文件分片上传（注意：当前后端不支持，这里只是示例）
  // 实际使用时需要后端支持分片上传接口
  const totalChunks = Math.ceil(file.size / chunkSize)
  let uploadedBytes = 0

  for (let i = 0; i < totalChunks; i++) {
    const start = i * chunkSize
    const end = Math.min(start + chunkSize, file.size)
    const chunk = file.slice(start, end)

    const formData = new FormData()
    Object.keys(additionalData).forEach((key) => {
      formData.append(key, additionalData[key])
    })
    formData.append('file', chunk, file.name)
    formData.append('chunk_index', i)
    formData.append('total_chunks', totalChunks)

    try {
      await uploadFile(
        url,
        formData,
        {},
        (percent) => {
          // 计算总体进度
          const chunkProgress = (i / totalChunks) * 100 + (percent / totalChunks)
          if (onProgress) {
            onProgress(Math.round(chunkProgress))
          }
        }
      )

      uploadedBytes += chunk.size
    } catch (error) {
      throw new Error(`分片 ${i + 1}/${totalChunks} 上传失败: ${error.message}`)
    }
  }

  return { success: true }
}

