export async function copyText(t: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(t)
    return true
  } catch {
    try {
      const ta = document.createElement('textarea')
      ta.value = t
      ta.setAttribute('readonly', '')
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      const ok = document.execCommand('copy')
      ta.remove()
      return ok
    } catch {
      return false
    }
  }
}

const UP = 'ABCDEFGHJKLMNPQRSTUVWXYZ'
const LOW = 'abcdefghijkmnopqrstuvwxyz'
const NUM = '23456789'

function rnd(n: number) {
  const lim = Math.floor(0xffffffff / n) * n
  const b = new Uint32Array(1)
  do crypto.getRandomValues(b)
  while (b[0] >= lim)
  return b[0] % n
}

export function randomPassword(len = 16): string {
  const all = UP + LOW + NUM
  const out = [UP[rnd(UP.length)], LOW[rnd(LOW.length)], NUM[rnd(NUM.length)]]
  while (out.length < len) out.push(all[rnd(all.length)])
  for (let i = out.length - 1; i > 0; i--) {
    const j = rnd(i + 1)
    ;[out[i], out[j]] = [out[j], out[i]]
  }
  return out.join('')
}

export function mergeLines(current: string, add: string[]): string {
  const seen = new Set<string>()
  const out: string[] = []
  for (const l of [...current.split('\n'), ...add]) {
    const t = l.trim()
    if (t && !seen.has(t)) {
      seen.add(t)
      out.push(t)
    }
  }
  return out.join('\n')
}
