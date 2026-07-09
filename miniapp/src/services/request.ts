export interface RequestOptions<T = unknown> {
  url: string
  method?: UniNamespace.RequestOptions['method']
  data?: T
  header?: Record<string, string>
  mock?: boolean
}

export interface ApiResponse<T> {
  code: number
  message: string
  data: T
}

export function request<T, D extends string | AnyObject | ArrayBuffer = AnyObject>(options: RequestOptions<D>) {
  return new Promise<T>((resolve, reject) => {
    uni.request({
      url: options.url,
      method: options.method || 'GET',
      data: options.data,
      header: options.header,
      success(response) {
        if (response.statusCode >= 200 && response.statusCode < 300) {
          const payload = response.data as Partial<ApiResponse<T>>
          resolve((payload.data ?? response.data) as T)
          return
        }
        reject(new Error(`HTTP ${response.statusCode}`))
      },
      fail(error) {
        reject(error)
      },
    })
  })
}
