import { createFileRoute } from '@tanstack/react-router'
import { Star } from 'lucide-react'
import { PlaceholderPage } from '../pages/placeholder-page'

export const Route = createFileRoute('/scores')({
  component: () => (
    <PlaceholderPage
      title='Scores'
      description='Review evaluation scores attached to your traces.'
      icon={Star}
    />
  ),
})
