import { createFileRoute } from '@tanstack/react-router'
import { Settings2 } from 'lucide-react'
import { PlaceholderPage } from '../pages/placeholder-page'

export const Route = createFileRoute('/settings')({
  component: () => (
    <PlaceholderPage
      title='Settings'
      description='Configure your local Pocketfuse workspace.'
      icon={Settings2}
    />
  ),
})
