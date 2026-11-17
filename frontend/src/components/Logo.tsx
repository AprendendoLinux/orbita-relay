import { useId } from 'react'

export default function Logo({ size = 40 }: { size?: number }) {
  const id = useId().replace(/:/g, '')
  return (
    <svg className="logo-mark" width={size} height={size} viewBox="0 0 64 64" role="img" aria-label="OrbitaRelay">
      <defs>
        <linearGradient id={`g${id}`} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="var(--primary)" />
          <stop offset="1" stopColor="var(--secondary)" />
        </linearGradient>
        <filter id={`f${id}`} x="-40%" y="-40%" width="180%" height="180%">
          <feGaussianBlur stdDeviation="2.2" result="b" />
          <feMerge><feMergeNode in="b" /><feMergeNode in="SourceGraphic" /></feMerge>
        </filter>
      </defs>
      <g filter={`url(#f${id})`}>
        <ellipse cx="32" cy="32" rx="26" ry="10.5" fill="none" stroke={`url(#g${id})`} strokeWidth="3" transform="rotate(-28 32 32)" />
        <ellipse cx="32" cy="32" rx="26" ry="10.5" fill="none" stroke={`url(#g${id})`} strokeWidth="1.2" opacity="0.45" transform="rotate(32 32 32)" />
        <circle cx="32" cy="32" r="9" fill={`url(#g${id})`} />
        <path d="M27.5 30.2h9M27.5 30.2l4.5 3.6 4.5-3.6M27.5 30.2v4.6h9v-4.6" fill="none" stroke="var(--logo-ink)" strokeWidth="1.3" strokeLinejoin="round" strokeLinecap="round" />
        <circle cx="54" cy="21.5" r="4" fill="var(--accent)" />
      </g>
    </svg>
  )
}
