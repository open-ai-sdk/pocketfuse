import { createFileRoute } from '@tanstack/react-router'
import { TraceDetailPage } from '../../pages/trace-detail-page'

export const Route = createFileRoute('/traces/$traceId')({
  component: TraceDetailPage,
})
