import { createFileRoute } from '@tanstack/react-router'
import { UsersRound } from 'lucide-react'
import { PlaceholderPage } from '../pages/placeholder-page'

export const Route = createFileRoute('/sessions')({
  component: () => (
    <PlaceholderPage
      title='Sessions'
      description='Group traces by conversations and user journeys.'
      icon={UsersRound}
    />
  ),
})
