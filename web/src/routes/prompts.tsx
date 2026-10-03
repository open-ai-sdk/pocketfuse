import { createFileRoute } from '@tanstack/react-router'
import { FileJson2 } from 'lucide-react'
import { PlaceholderPage } from '../pages/placeholder-page'

export const Route = createFileRoute('/prompts')({
  component: () => (
    <PlaceholderPage
      title='Prompts'
      description='Inspect and version prompts used by your agents.'
      icon={FileJson2}
    />
  ),
})
