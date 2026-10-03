import { formatDate } from '../../lib/utils'
import type { Score } from '../../lib/api'

// Scores tab content: one row per score, readable at a glance — name,
// value, source, comment and when it landed.
export function ScoreRows({
  scores,
  emptyHint = 'No scores recorded yet.',
}: {
  scores: Score[]
  emptyHint?: string
}) {
  if (scores.length === 0) {
    return <p className='px-1 py-8 text-center text-[11px] text-content-tertiary'>{emptyHint}</p>
  }
  return (
    <div className='divide-y divide-border-primary'>
      {scores.map((score) => (
        <div
          key={score.id}
          className='grid grid-cols-[minmax(90px,160px)_minmax(72px,auto)_minmax(0,1fr)_auto] items-baseline gap-x-4 px-1 py-2'
        >
          <span className='truncate text-xs font-medium text-content-primary'>
            {score.name || 'score'}
          </span>
          <span className='font-mono text-xs tabular-nums text-content-primary'>
            {score.value !== undefined ? score.value.toFixed(2) : (score.stringValue ?? '—')}
          </span>
          <span className='min-w-0 truncate text-[11px] text-content-secondary'>
            {score.comment || (score.source ? `source: ${score.source}` : '—')}
          </span>
          <span className='whitespace-nowrap text-[10px] text-content-tertiary'>
            {score.timestamp ? formatDate(score.timestamp) : ''}
          </span>
        </div>
      ))}
    </div>
  )
}
