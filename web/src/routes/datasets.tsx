import { createFileRoute } from '@tanstack/react-router'
import { Boxes } from 'lucide-react'
import { PlaceholderPage } from '../pages/placeholder-page'

export const Route = createFileRoute('/datasets')({
  component: () => (
    <PlaceholderPage
      title='Datasets'
      description='Manage local evaluation datasets.'
      icon={Boxes}
    />
  ),
})
