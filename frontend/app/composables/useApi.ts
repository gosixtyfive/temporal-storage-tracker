import { createApi, type Api, type Fetcher } from '~/utils/api'

let api: Api | undefined

/** The API client, backed by Nuxt's $fetch. Requests go to /api/**, which Nuxt proxies to the Go server. */
export function useApi(): Api {
  // FetchOptions is a subset of $fetch's options; its body is a plain JSON object.
  api ??= createApi(((url, options) => $fetch(url, options as Parameters<typeof $fetch>[1])) as Fetcher)
  return api
}
