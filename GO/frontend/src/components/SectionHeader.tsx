import type { ReactNode } from 'react'

export function SectionHeader({
  index,
  title,
  action,
}: {
  index: string
  title: string
  action?: ReactNode
}) {
  return (
    <div className="mb-2.5 flex items-center justify-between border-b border-border pb-2.5">
      <h2 className="flex items-baseline gap-2 text-xs font-extrabold uppercase tracking-wider text-ink">
        <span className="font-mono font-semibold tracking-normal text-brandPurple">{index}</span>
        {title}
      </h2>
      {action}
    </div>
  )
}
