import { createFileRoute } from '@tanstack/react-router'
import { BarChart3 } from 'lucide-react'
import { PlaceholderPage } from '../pages/placeholder-page'

export const Route = createFileRoute('/models')({
  component: () => (
    <PlaceholderPage
      title='Models & costs'
      description='Understand model usage and spend over time.'
      icon={BarChart3}
    />
  ),
})
