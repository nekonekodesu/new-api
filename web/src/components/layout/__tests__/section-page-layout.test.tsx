/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  getCoreRowModel,
  getFilteredRowModel,
  useReactTable,
  type ColumnDef,
  type RowSelectionState,
} from '@tanstack/react-table'
import { render, screen, within } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { DataTableBulkActions, DataTablePage } from '@/components/data-table'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { SectionPageLayout } from '../components/section-page-layout'

type ChannelRow = {
  id: number
  name: string
}

const CHANNEL_COLUMNS: ColumnDef<ChannelRow, unknown>[] = [
  { accessorKey: 'id', header: 'ID' },
  { accessorKey: 'name', header: 'Channel Name' },
]

const MANY_ROWS: ChannelRow[] = Array.from({ length: 81 }, (_, index) => ({
  id: index + 1,
  name: `Channel ${index + 1}`,
}))

function setCompactViewport(compact: boolean) {
  const original = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...original(query),
    matches: compact && query.includes('max-width'),
  }))
}

function FixedTableWithTabsFixture(props: {
  activeTab: 'all' | 'groups'
  rows?: ChannelRow[]
}) {
  const table = useReactTable({
    data: props.rows ?? MANY_ROWS,
    columns: CHANNEL_COLUMNS,
    getCoreRowModel: getCoreRowModel(),
  })

  return (
    <SectionPageLayout
      fixedContent={props.activeTab === 'all'}
      stackActionsOnMobile={props.activeTab === 'all'}
    >
      <SectionPageLayout.Title>Channels</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <button type='button'>Add Channel</button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <Tabs value={props.activeTab} className='mb-3 w-full shrink-0'>
          <TabsList>
            <TabsTrigger value='all'>All Channels</TabsTrigger>
            <TabsTrigger value='groups'>Groups</TabsTrigger>
          </TabsList>
        </Tabs>
        <div data-testid='tab-body' className='flex min-h-0 flex-1 flex-col'>
          {props.activeTab === 'all' ? (
            <div
              role='region'
              aria-label='Channels'
              className='relative h-full min-h-0'
            >
              <DataTablePage
                table={table}
                columns={CHANNEL_COLUMNS}
                emptyTitle='No Channels Found'
              />
            </div>
          ) : (
            <section aria-label='Group Management'>
              Group settings content
            </section>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function DirectTableAndWrapperCallersFixture(props: {
  mode: 'direct-table' | 'banner-wrapper' | 'tabs-content'
}) {
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({
    '0': true,
  })
  const table = useReactTable({
    data: MANY_ROWS,
    columns: CHANNEL_COLUMNS,
    state: { rowSelection },
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
  })

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>Management</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {props.mode === 'direct-table' && (
          <DataTablePage
            table={table}
            columns={CHANNEL_COLUMNS}
            emptyTitle='No Records'
            bulkActions={
              <DataTableBulkActions table={table} entityName='record'>
                <button type='button'>Delete</button>
              </DataTableBulkActions>
            }
          />
        )}
        {props.mode === 'banner-wrapper' && (
          <div
            data-testid='banner-wrapper'
            className='flex h-full min-h-0 flex-col gap-4'
          >
            <Alert variant='destructive' className='shrink-0'>
              <AlertDescription>Compliance notice</AlertDescription>
            </Alert>
            <div className='min-h-0 flex-1'>
              <DataTablePage
                table={table}
                columns={CHANNEL_COLUMNS}
                emptyTitle='No Plans'
              />
            </div>
          </div>
        )}
        {props.mode === 'tabs-content' && (
          <Tabs
            value='installed'
            className='flex h-full min-h-0 flex-col gap-4'
          >
            <div className='shrink-0 border-b'>
              <TabsList variant='line'>
                <TabsTrigger value='installed'>Installed</TabsTrigger>
              </TabsList>
            </div>
            <TabsContent value='installed' className='min-h-0 flex-1'>
              <DataTablePage
                table={table}
                columns={CHANNEL_COLUMNS}
                emptyTitle='No Plugins'
              />
            </TabsContent>
          </Tabs>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('SectionPageLayout fixedContent scrolling contract', () => {
  it('establishes a vertical flex container when fixedContent is true so sibling Tabs and flex-1 table regions constrain height to the inner table scroll wrapper', () => {
    setCompactViewport(false)
    render(<FixedTableWithTabsFixture activeTab='all' />)

    const main = screen.getByRole('main')
    expect(main).toHaveClass(
      'flex',
      'flex-col',
      'min-h-0',
      'flex-1',
      'overflow-hidden'
    )

    const tablist = screen.getByRole('tablist')
    const tabBody = screen.getByTestId('tab-body')
    const contentContainer = tabBody.parentElement
    expect(contentContainer).not.toBeNull()
    expect(contentContainer?.parentElement).toBe(main)

    expect(contentContainer).toHaveClass(
      'flex',
      'flex-col',
      'min-h-0',
      'flex-1',
      'overflow-hidden'
    )
    expect(contentContainer).not.toHaveClass('overflow-auto')
    expect(tablist.closest('[data-slot="tabs"]')).toHaveClass('shrink-0')

    const tableRegion = screen.getByRole('region', { name: 'Channels' })
    const table = within(tableRegion).getByRole('table')
    const thead = table.querySelector('thead')
    const tableScrollWrapper = table.parentElement

    expect(thead).toHaveClass('sticky', 'top-0')
    expect(tableScrollWrapper).toHaveClass('min-h-0', 'flex-1', 'overflow-auto')
    expect(within(table).getAllByRole('row')).toHaveLength(MANY_ROWS.length + 1)
  })

  it('renders empty state cleanly inside the bounded scroll wrapper when rows are empty', () => {
    setCompactViewport(false)
    render(<FixedTableWithTabsFixture activeTab='all' rows={[]} />)

    expect(screen.getByText('No Channels Found')).toBeVisible()
    const table = screen.getByRole('table')
    expect(table.parentElement).toHaveClass(
      'min-h-0',
      'flex-1',
      'overflow-auto'
    )
  })

  it('switches cleanly to outer overflow-auto scrolling without flex-col when fixedContent becomes false on non-table tabs', () => {
    setCompactViewport(false)
    const { rerender } = render(<FixedTableWithTabsFixture activeTab='all' />)

    const initialContent = screen.getByTestId('tab-body').parentElement
    expect(initialContent).toHaveClass('flex', 'flex-col', 'overflow-hidden')

    rerender(<FixedTableWithTabsFixture activeTab='groups' />)

    const groupSection = screen.getByRole('region', {
      name: 'Group Management',
    })
    const updatedContent = screen.getByTestId('tab-body').parentElement
    expect(groupSection).toBeVisible()
    expect(updatedContent).toHaveClass('min-h-0', 'flex-1', 'overflow-auto')
    expect(updatedContent).not.toHaveClass('overflow-hidden')
    expect(updatedContent).not.toHaveClass('flex-col')
  })

  it.each([
    { mode: 'direct-table' as const },
    { mode: 'banner-wrapper' as const },
    { mode: 'tabs-content' as const },
  ])(
    'preserves bounded inner table scroll wrapper and out-of-flow bulk actions for $mode fixedContent pages',
    ({ mode }) => {
      setCompactViewport(false)
      render(<DirectTableAndWrapperCallersFixture mode={mode} />)

      const table = screen.getByRole('table')
      const thead = table.querySelector('thead')
      const tableScrollWrapper = table.parentElement
      expect(thead).toHaveClass('sticky', 'top-0')
      expect(tableScrollWrapper).toHaveClass(
        'min-h-0',
        'flex-1',
        'overflow-auto'
      )

      if (mode === 'direct-table') {
        const toolbar = screen.getByRole('toolbar')
        expect(toolbar).toHaveClass('fixed')
      }
    }
  )

  it('keeps header and portaled footer outside the fixedContent region and delegates mobile scrolling to the inner list wrapper', () => {
    setCompactViewport(true)
    render(<FixedTableWithTabsFixture activeTab='all' />)

    const main = screen.getByRole('main')
    const heading = screen.getByRole('heading', { name: 'Channels' })
    const headerWrapper = heading.closest('main > div')
    const contentContainer = screen.getByTestId('tab-body').parentElement
    const footerWrapper = main.lastElementChild

    expect(headerWrapper).toHaveClass('shrink-0')
    expect(heading.parentElement).toHaveClass('max-sm:basis-full')
    expect(contentContainer).toHaveClass(
      'flex',
      'flex-col',
      'min-h-0',
      'flex-1',
      'overflow-hidden'
    )
    expect(footerWrapper).not.toBe(contentContainer)
    expect(footerWrapper).toHaveClass('shrink-0')

    const tableRegion = screen.getByRole('region', { name: 'Channels' })
    const mobileScrollContainer = tableRegion.querySelector('.overflow-y-auto')
    expect(mobileScrollContainer).not.toBeNull()
    expect(mobileScrollContainer).toHaveClass(
      'min-h-0',
      'flex-1',
      'overflow-y-auto'
    )
  })
})
