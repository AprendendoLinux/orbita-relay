import type { ReactNode } from 'react'

export default function Tip({ title, children }: { title?: string; children: ReactNode }) {
  return (
    <div className="tip" role="note">
      {title && <b>{title}</b>}
      {children}
    </div>
  )
}
