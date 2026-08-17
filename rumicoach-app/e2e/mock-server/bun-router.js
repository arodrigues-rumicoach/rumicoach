// Bun-compatible router that mimics Express.Router() structure
export function createRouter() {
  const stack = []

  function addRoute(method, path, ...handlers) {
    stack.push({
      route: {
        path,
        methods: { [method.toLowerCase()]: handlers },
      },
    })
  }

  return {
    stack,
    get: (path, ...handlers) => addRoute('get', path, ...handlers),
    post: (path, ...handlers) => addRoute('post', path, ...handlers),
    put: (path, ...handlers) => addRoute('put', path, ...handlers),
    patch: (path, ...handlers) => addRoute('patch', path, ...handlers),
    delete: (path, ...handlers) => addRoute('delete', path, ...handlers),
    use: () => {},
  }
}

// Execute middleware chain with proper async/await support.
// Middleware calls next() to proceed, or returns without calling next() to stop.
export async function runMiddlewareChain(handlers, req, res) {
  let index = -1

  async function next() {
    index++
    if (index >= handlers.length) return
    if (res.finished) return

    const handler = handlers[index]
    try {
      const result = handler(req, res, next)
      // If handler returns a Promise (async), await it
      if (result && typeof result.then === 'function') {
        await result
      }
    } catch (e) {
      if (!res.finished) {
        res.status(500).json({ code: 'INTERNAL_ERROR', error: e.message })
      }
    }
  }

  await next()
}
