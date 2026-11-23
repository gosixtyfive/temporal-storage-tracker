// One shared clock for every countdown on the page, ticking once a second
// while at least one component is using it.
const now = ref(Date.now())
let users = 0
let timer: ReturnType<typeof setInterval> | undefined

/** The current time in ms, updated every second. Use inside a component's setup. */
export function useNow(): Readonly<Ref<number>> {
  if (users++ === 0) {
    now.value = Date.now()
    timer = setInterval(() => {
      now.value = Date.now()
    }, 1000)
  }
  onScopeDispose(() => {
    if (--users === 0) {
      clearInterval(timer)
      timer = undefined
    }
  })
  return readonly(now)
}
