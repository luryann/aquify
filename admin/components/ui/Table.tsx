'use client'

import { useState, useMemo, useCallback, type ReactNode, type CSSProperties } from 'react'
import { Icon } from './icons'
import { EmptyState } from './EmptyState'
import { SkeletonTable } from './Skeleton'
import './Table.inline.css'

// ─────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────

export type SortDirection = 'asc' | 'desc'

export type SortState = {
  key: string
  direction: SortDirection
}

export type ColumnDef<T> = {
  key: string
  label: string
  sortable?: boolean
  width?: string
  align?: 'left' | 'center' | 'right'
  render?: (row: T, index: number) => ReactNode
  headerStyle?: CSSProperties
  cellStyle?: CSSProperties
}

// Convenience alias — prefer Column<T> in new code
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type Column<T extends Record<string, any> = Record<string, unknown>> = ColumnDef<T>

export type TableProps<T extends Record<string, unknown>> = {
  columns: ColumnDef<T>[]
  data: T[]
  keyField?: keyof T
  loading?: boolean
  emptyTitle?: string
  emptyDescription?: string

  // Sorting
  sortable?: boolean
  defaultSort?: SortState
  onSortChange?: (sort: SortState | null) => void

  // Pagination
  paginated?: boolean
  pageSize?: number
  totalItems?: number    // for server-side
  currentPage?: number   // for server-side
  onPageChange?: (page: number) => void

  // Selection
  selectable?: boolean
  selectedKeys?: Set<string>
  onSelectionChange?: (keys: Set<string>) => void

  // Row actions
  onRowClick?: (row: T) => void
  /** Key (per keyField) of the row showing the keyboard-focus ring. */
  highlightKey?: string

  // Style
  compact?: boolean
  stickyHeader?: boolean
  style?: CSSProperties
  className?: string
}

// ─────────────────────────────────────────────
// Pagination
// ─────────────────────────────────────────────

type PaginationProps = {
  page: number
  totalPages: number
  onPageChange: (page: number) => void
}

function Pagination({ page, totalPages, onPageChange }: PaginationProps) {
  const pages = useMemo(() => {
    const range: (number | '…')[] = []
    if (totalPages <= 7) {
      for (let i = 1; i <= totalPages; i++) range.push(i)
    } else {
      range.push(1)
      if (page > 3) range.push('…')
      for (let i = Math.max(2, page - 1); i <= Math.min(totalPages - 1, page + 1); i++) {
        range.push(i)
      }
      if (page < totalPages - 2) range.push('…')
      range.push(totalPages)
    }
    return range
  }, [page, totalPages])

  if (totalPages <= 1) return null

  const btnStyle = (active?: boolean, disabled?: boolean): CSSProperties => ({
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    minWidth: '30px',
    height: '30px',
    padding: '0 8px',
    borderRadius: 'var(--r-sm)',
    border: active ? '1px solid var(--accent-border)' : '1px solid transparent',
    background: active ? 'var(--accent-subtle)' : 'transparent',
    color: active ? 'var(--accent)' : disabled ? 'var(--text-muted)' : 'var(--text-subtle)',
    fontSize: 'var(--fs-sm)',
    fontFamily: 'var(--font-body)',
    fontWeight: active ? 600 : 400,
    cursor: disabled ? 'not-allowed' : 'pointer',
    transition: 'background var(--dur-fast), color var(--dur-fast), border-color var(--dur-fast)',
    userSelect: 'none',
  })

  return (
    <div
      className="u-p-12px-16px u-gap-8px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderTop: '1px solid var(--border)', flexWrap: 'wrap' }}
    >
      <span className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
        Page {page} of {totalPages}
      </span>

      <div className="u-gap-4px" style={{ display: 'flex', alignItems: 'center' }}>
        <button type="button"
          onClick={() => onPageChange(page - 1)}
          disabled={page <= 1}
          style={btnStyle(false, page <= 1)}
          aria-label="Previous page"
        >
          <Icon name="chevron-left" size={13} />
        </button>

        {pages.map((p, i) =>
          p === '…' ? (
            <span key={`ellipsis-${i}`} style={{ ...btnStyle(), cursor: 'default' }}>…</span>
          ) : (
            <button type="button"
              key={p}
              onClick={() => onPageChange(p)}
              style={btnStyle(p === page)}
            >
              {p}
            </button>
          ),
        )}

        <button type="button"
          onClick={() => onPageChange(page + 1)}
          disabled={page >= totalPages}
          style={btnStyle(false, page >= totalPages)}
          aria-label="Next page"
        >
          <Icon name="chevron-right" size={13} />
        </button>
      </div>
    </div>
  )
}

// ─────────────────────────────────────────────
// Column header
// ─────────────────────────────────────────────

function ColumnHeader<T>({
  col,
  sort,
  onSort,
}: {
  col: ColumnDef<T>
  sort: SortState | null
  onSort: (key: string) => void
}) {
  const isActive = sort?.key === col.key
  const dir = isActive ? sort!.direction : null

  const style: CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
    justifyContent: col.align === 'right' ? 'flex-end' : col.align === 'center' ? 'center' : 'flex-start',
    cursor: col.sortable ? 'pointer' : 'default',
    userSelect: 'none',
    width: '100%',
    color: isActive ? 'var(--text)' : 'var(--text-subtle)',
    transition: 'color var(--dur-fast)',
  }

  return (
    <div
      style={style}
      onClick={col.sortable ? () => onSort(col.key) : undefined}
    >
      <span className="col-head" style={{ color: 'inherit' }}>
        {col.label}
      </span>
      {col.sortable && (
        <span style={{ opacity: isActive ? 1 : 0.4, flexShrink: 0 }}>
          <Icon name={dir === 'asc' ? 'arrow-up' : 'arrow-down'} size={11} />
        </span>
      )}
    </div>
  )
}

// ─────────────────────────────────────────────
// Main Table component
// ─────────────────────────────────────────────

export function Table<T extends Record<string, unknown>>({
  columns,
  data,
  keyField,
  loading = false,
  emptyTitle = 'No data',
  emptyDescription,

  sortable = true,
  defaultSort,
  onSortChange,

  paginated = false,
  pageSize = 20,
  totalItems,
  currentPage: controlledPage,
  onPageChange: controlledOnPageChange,

  selectable = false,
  selectedKeys,
  onSelectionChange,

  onRowClick,
  highlightKey,

  compact = false,
  stickyHeader = false,
  style,
  className,
}: TableProps<T>) {
  const [sort, setSort] = useState<SortState | null>(defaultSort ?? null)
  const [internalPage, setInternalPage] = useState(1)

  const page = controlledPage ?? internalPage
  const isServerPaginated = controlledPage !== undefined

  // ── Sort handler
  const handleSort = useCallback(
    (key: string) => {
      const next: SortState =
        sort?.key === key
          ? { key, direction: sort.direction === 'asc' ? 'desc' : 'asc' }
          : { key, direction: 'asc' }
      setSort(next)
      onSortChange?.(next)
    },
    [sort, onSortChange],
  )

  // ── Client-side sort
  const sortedData = useMemo(() => {
    if (!sort || isServerPaginated) return data
    return [...data].sort((a, b) => {
      const av = a[sort.key]
      const bv = b[sort.key]
      const aStr = av === null || av === undefined ? '' : String(av)
      const bStr = bv === null || bv === undefined ? '' : String(bv)
      const cmp = aStr.localeCompare(bStr, undefined, { numeric: true, sensitivity: 'base' })
      return sort.direction === 'asc' ? cmp : -cmp
    })
  }, [data, sort, isServerPaginated])

  // ── Client-side pagination
  const totalCount = totalItems ?? sortedData.length
  const totalPages = Math.max(1, Math.ceil(totalCount / pageSize))

  const displayData = useMemo(() => {
    if (!paginated || isServerPaginated) return sortedData
    const start = (page - 1) * pageSize
    return sortedData.slice(start, start + pageSize)
  }, [sortedData, paginated, isServerPaginated, page, pageSize])

  const handlePageChange = useCallback(
    (p: number) => {
      if (controlledOnPageChange) controlledOnPageChange(p)
      else setInternalPage(p)
    },
    [controlledOnPageChange],
  )

  // ── Selection
  const allKeys = displayData.map((row, i) =>
    keyField ? String(row[keyField]) : String(i),
  )
  const allSelected = allKeys.length > 0 && allKeys.every((k) => selectedKeys?.has(k))
  const someSelected = !allSelected && allKeys.some((k) => selectedKeys?.has(k))

  const handleSelectAll = useCallback(() => {
    if (!onSelectionChange) return
    if (allSelected) {
      onSelectionChange(new Set([...(selectedKeys ?? [])].filter((k) => !allKeys.includes(k))))
    } else {
      onSelectionChange(new Set([...(selectedKeys ?? []), ...allKeys]))
    }
  }, [allSelected, allKeys, selectedKeys, onSelectionChange])

  const handleSelectRow = useCallback(
    (key: string) => {
      if (!onSelectionChange || !selectedKeys) return
      const next = new Set(selectedKeys)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      onSelectionChange(next)
    },
    [onSelectionChange, selectedKeys],
  )

  // ── Styles
  const rowPadding = compact ? '8px 16px' : '11px 16px'
  const thPadding  = compact ? '6px 16px' : '9px 16px'

  // ── Loading state
  if (loading) {
    return (
      <div
        style={{
          border: '1px solid var(--border)',
          borderRadius: 'var(--r-lg)',
          overflow: 'hidden',
          background: 'var(--bg-surface)',
          ...style,
        }}
        className={className}
      >
        <SkeletonTable rows={5} columns={columns.length} />
      </div>
    )
  }

  return (
    <div
      style={{
        border: '1px solid var(--border)',
        borderRadius: 'var(--r-lg)',
        overflow: 'hidden',
        background: 'var(--bg-surface)',
        display: 'flex',
        flexDirection: 'column',
        ...style,
      }}
      className={className}
    >
      {/* Table scroll wrapper */}
      <div className="table-scroll" style={{ flex: 1 }}>
        <table
          style={{
            width: '100%',
            borderCollapse: 'collapse',
            minWidth: '480px',
          }}
        >
          <thead style={stickyHeader ? { position: 'sticky', top: 0, zIndex: 1, background: 'var(--bg-surface)' } : undefined}>
            <tr style={{ borderBottom: '1px solid var(--border)' }}>
              {selectable && (
                <th style={{ width: '40px', padding: thPadding, textAlign: 'center' }}>
                  <input
                    type="checkbox"
                    checked={allSelected}
                    ref={(el) => { if (el) el.indeterminate = someSelected }}
                    onChange={handleSelectAll}
                    style={{ accentColor: 'var(--accent)' }}
                    aria-label="Select all"
                  />
                </th>
              )}
              {columns.map((col) => (
                <th
                  key={col.key}
                  style={{
                    padding: thPadding,
                    width: col.width,
                    textAlign: col.align ?? 'left',
                    background: 'inherit',
                    ...col.headerStyle,
                  }}
                >
                  {sortable && col.sortable !== false ? (
                    <ColumnHeader col={col} sort={sort} onSort={handleSort} />
                  ) : (
                    <span className="col-head">
                      {col.label}
                    </span>
                  )}
                </th>
              ))}
            </tr>
          </thead>

          <tbody>
            {displayData.map((row, rowIdx) => {
                const rowKey = keyField ? String(row[keyField]) : String(rowIdx)
                const isSelected = selectedKeys?.has(rowKey) ?? false

                return (
                  <tr
                    key={rowKey}
                    className={highlightKey !== undefined && rowKey === highlightKey ? 'table-row row-focus' : 'table-row'}
                    data-selected={isSelected}
                    ref={highlightKey !== undefined && rowKey === highlightKey ? (el) => el?.scrollIntoView({ block: 'nearest' }) : undefined}
                    onClick={onRowClick ? () => onRowClick(row) : undefined}
                    style={{
                      borderBottom: rowIdx < displayData.length - 1 ? '1px solid var(--border)' : 'none',
                      cursor: onRowClick ? 'pointer' : 'default',
                    }}
                  >
                    {selectable && (
                      <td
                        style={{ padding: rowPadding, textAlign: 'center', width: '40px' }}
                        onClick={(e) => { e.stopPropagation(); handleSelectRow(rowKey) }}
                      >
                        <input
                          type="checkbox"
                          checked={isSelected}
                          onChange={() => handleSelectRow(rowKey)}
                          style={{ accentColor: 'var(--accent)' }}
                          aria-label={`Select row ${rowIdx + 1}`}
                        />
                      </td>
                    )}
                    {columns.map((col) => {
                      const value = row[col.key]
                      return (
                        <td
                          key={col.key}
                          style={{
                            padding: rowPadding,
                            textAlign: col.align ?? 'left',
                            fontSize: 'var(--fs-base)',
                            color: 'var(--text)',
                            verticalAlign: 'middle',
                            ...col.cellStyle,
                          }}
                        >
                          {col.render
                            ? col.render(row, rowIdx)
                            : value === null || value === undefined
                              ? <span style={{ color: 'var(--text-muted)' }}>—</span>
                              : String(value)}
                        </td>
                      )
                    })}
                  </tr>
                )
              })}
          </tbody>
        </table>
      </div>

      {/* Empty state sits outside the horizontal scroller so it stays centered
          in the visible area when the columns are wider than the screen. */}
      {displayData.length === 0 && (
        <EmptyState
          title={emptyTitle}
          description={emptyDescription}
          size="sm"
          bordered={false}
        />
      )}

      {/* Pagination */}
      {paginated && totalPages > 1 && (
        <Pagination page={page} totalPages={totalPages} onPageChange={handlePageChange} />
      )}
    </div>
  )
}

// ─────────────────────────────────────────────
// Re-export helpers for column building
// ─────────────────────────────────────────────

export function col<T extends Record<string, unknown>>(def: ColumnDef<T>): ColumnDef<T> {
  return def
}
