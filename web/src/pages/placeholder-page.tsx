import type { LucideIcon } from 'lucide-react'
import { ArrowRight, Construction } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { Button } from '../components/ui'
import { panelSurface, PageHeader, EmptyPanel } from './primitives'

export function PlaceholderPage({
  title,
  description,
  icon: Icon = Construction,
}: {
  title: string
  description: string
  icon?: LucideIcon
}) {
  return (
    <div className='min-h-full bg-background-primary text-content-primary'>
      <PageHeader eyebrow='Workspace' title={title} description={description} />
      <div className='px-4 py-5 sm:px-6'>
        <div className={panelSurface}>
          <EmptyPanel
            icon={<Icon className='size-4' />}
            title={`${title} is ready for your data`}
            description='This area will be connected to the local API as the MVP grows.'
            action={
              <Button render={<Link to='/traces' />} variant='outline' size='sm'>
                View traces
                <ArrowRight data-icon='inline-end' />
              </Button>
            }
          />
        </div>
      </div>
    </div>
  )
}
