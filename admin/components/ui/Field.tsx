import { cloneElement, isValidElement, useId, type ReactElement, type ReactNode } from 'react'

type FieldProps = {
  label: string
  hint?: string
  error?: string | null
  required?: boolean
  children: ReactElement<Record<string, unknown>>
  className?: string
}

/** Label + control + hint/error wired with htmlFor/id and aria-describedby. */
export function Field({ label, hint, error, required, children, className }: FieldProps) {
  const uid = useId()
  const id = (isValidElement(children) && (children.props.id as string | undefined)) || `f-${uid}`
  const hintId = hint ? `${id}-hint` : undefined
  const errId = error ? `${id}-err` : undefined
  const describedBy = [hintId, errId].filter(Boolean).join(' ') || undefined
  const control: ReactNode = cloneElement(children, {
    id,
    'aria-describedby': describedBy,
    'aria-invalid': error ? true : undefined,
    'aria-required': required ? true : undefined,
  })
  return (
    <div className={className}>
      <label htmlFor={id} className={`form-label${required ? ' form-label-required' : ''}`}>
        {label}
      </label>
      {control}
      {hint && !error && (
        <p id={hintId} className="field-hint">
          {hint}
        </p>
      )}
      {error && (
        <p id={errId} role="alert" className="field-error">
          {error}
        </p>
      )}
    </div>
  )
}
