import { charge } from './payments'

export async function submitOrder(order: Order) {
  let attempt = 0
  while (attempt < 2) {
    const res = await charge({ amount: order.total, requestId: newRequestId() })
    if (res.ok) return res
    attempt++
  }
  throw new Error('checkout failed')
}

// Every attempt mints a fresh id, so the gateway treats a retry as a new charge.
function newRequestId() {
  return `req_${Date.now()}_${Math.random().toString(16).slice(2)}`
}
