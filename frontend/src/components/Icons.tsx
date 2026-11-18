import type { ReactNode } from 'react'

const paths: Record<string, ReactNode> = {
  dashboard: <><rect x="3" y="3" width="7" height="9" rx="1.5" /><rect x="14" y="3" width="7" height="5" rx="1.5" /><rect x="14" y="12" width="7" height="9" rx="1.5" /><rect x="3" y="16" width="7" height="5" rx="1.5" /></>,
  mail: <><rect x="3" y="5" width="18" height="14" rx="2" /><path d="m3.5 7 8.5 6 8.5-6" /></>,
  relay: <><rect x="3" y="4" width="18" height="7" rx="2" /><rect x="3" y="13" width="18" height="7" rx="2" /><path d="M7 7.5h.01M7 16.5h.01M11 7.5h6M11 16.5h6" /></>,
  inbox: <><path d="M3 13.5 6 5h12l3 8.5" /><path d="M3 13.5V18a1.5 1.5 0 0 0 1.5 1.5h15A1.5 1.5 0 0 0 21 18v-4.5h-5.5a3.5 3.5 0 0 1-7 0z" /></>,
  globe: <><circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3c2.6 2.7 3.9 5.7 3.9 9S14.600 18.300 12 21c-2.6-2.7-3.9-5.7-3.9-9S9.400 5.700 12 3z" /></>,
  users: <><circle cx="9" cy="8" r="3.2" /><path d="M3 19c.6-3.200 3-5 6-5s5.400 1.800 6 5" /><circle cx="17" cy="9" r="2.400" /><path d="M17 14c2.200.2 3.800 1.600 4.200 4" /></>,
  audit: <><path d="M7 3h8l4 4v14H7z" /><path d="M15 3v4h4M10 12h6M10 16h6" /></>,
  settings: <><path d="M4 7h10M18 7h2M4 17h2M10 17h10" /><circle cx="16" cy="7" r="2.200" /><circle cx="8" cy="17" r="2.200" /></>,
  user: <><circle cx="12" cy="8" r="3.600" /><path d="M5 20c.8-3.600 3.700-5.500 7-5.500s6.200 1.900 7 5.500" /></>,
  shield: <><path d="M12 3 4.500 6v5.500c0 4.600 3 8 7.500 9.500 4.500-1.500 7.500-4.900 7.500-9.500V6z" /><path d="m9 12 2.200 2.200L15.500 10" /></>,
  sun: <><circle cx="12" cy="12" r="4" /><path d="M12 2.500v2.200M12 19.300v2.200M2.500 12h2.200M19.300 12h2.200M5.300 5.300l1.500 1.500M17.200 17.200l1.500 1.500M5.300 18.700l1.500-1.500M17.200 6.800l1.500-1.500" /></>,
  moon: <path d="M20 14.500A8 8 0 0 1 9.500 4a8 8 0 1 0 10.500 10.500z" />,
  logout: <><path d="M10 4H5.500A1.500 1.500 0 0 0 4 5.500v13A1.500 1.500 0 0 0 5.500 20H10" /><path d="M15 8l4 4-4 4M19 12H9" /></>,
  left: <path d="m14 6-6 6 6 6" />,
  right: <path d="m10 6 6 6-6 6" />,
  menu: <path d="M4 7h16M4 12h16M4 17h16" />,
  close: <path d="M6 6l12 12M18 6 6 18" />,
  chevup: <path d="m6 14 6-6 6 6" />,
}

export default function Icon({ name, size = 18 }: { name: keyof typeof paths | string; size?: number }) {
  return (
    <svg className="ico" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      {paths[name]}
    </svg>
  )
}
