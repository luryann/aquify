'use client'

import {
  useForm,
  FormProvider,
  useFormContext,
  Controller,
  type UseFormProps,
  type FieldValues,
  type SubmitHandler,
  type Path,
} from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import type { ZodType } from 'zod'
import type { ReactNode, CSSProperties } from 'react'
import { Icon } from '@/components/ui/icons'

// ─────────────────────────────────────────────
// Form wrapper
// ─────────────────────────────────────────────

type FormWrapperProps<T extends FieldValues> = {
  schema: ZodType<T>
  defaultValues?: UseFormProps<T>['defaultValues']
  onSubmit: SubmitHandler<T>
  onError?: (errors: unknown) => void
  children: ReactNode
  style?: CSSProperties
  className?: string
  id?: string
  mode?: UseFormProps<T>['mode']
}

export function FormWrapper<T extends FieldValues>({
  schema,
  defaultValues,
  onSubmit,
  onError,
  children,
  style,
  className,
  id,
  mode = 'onBlur',
}: FormWrapperProps<T>) {
  const methods = useForm<T>({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    resolver: zodResolver(schema as any),
    defaultValues,
    mode,
  })

  return (
    <FormProvider {...methods}>
      <form
        id={id}
        onSubmit={methods.handleSubmit(onSubmit, onError)}
        style={{ display: 'flex', flexDirection: 'column', gap: '16px', ...style }}
        className={className}
        noValidate
      >
        {children}
      </form>
    </FormProvider>
  )
}

// ─────────────────────────────────────────────
// Field — wraps label, input, hint, error
// ─────────────────────────────────────────────

type FieldProps<T extends FieldValues> = {
  name: Path<T>
  label?: string
  hint?: string
  required?: boolean
  children: (field: {
    id: string
    'aria-invalid': boolean
    'aria-describedby'?: string
  }) => ReactNode
}

export function Field<T extends FieldValues>({
  name,
  label,
  hint,
  required,
  children,
}: FieldProps<T>) {
  const { formState } = useFormContext<T>()
  const error = formState.errors[name]
  const errorMsg = error?.message as string | undefined
  const hintId = hint ? `${name}-hint` : undefined
  const errId  = errorMsg ? `${name}-error` : undefined
  const describedBy = [hintId, errId].filter(Boolean).join(' ') || undefined

  return (
    <div className="form-field">
      {label && (
        <label
          htmlFor={name}
          className={`form-label${required ? ' form-label-required' : ''}`}
        >
          {label}
        </label>
      )}
      {children({
        id: name,
        'aria-invalid': !!errorMsg,
        'aria-describedby': describedBy,
      })}
      {hint && !errorMsg && (
        <p id={hintId} className="form-hint">
          {hint}
        </p>
      )}
      {errorMsg && (
        <p id={errId} className="form-error" role="alert">
          <Icon name="alert-circle" size={12} />
          {errorMsg}
        </p>
      )}
    </div>
  )
}

// ─────────────────────────────────────────────
// TextField
// ─────────────────────────────────────────────

type TextFieldProps<T extends FieldValues> = {
  name: Path<T>
  label?: string
  hint?: string
  required?: boolean
  placeholder?: string
  type?: 'text' | 'email' | 'password' | 'number' | 'tel' | 'url'
  disabled?: boolean
  autoComplete?: string
}

export function TextField<T extends FieldValues>({
  name,
  label,
  hint,
  required,
  placeholder,
  type = 'text',
  disabled,
  autoComplete,
}: TextFieldProps<T>) {
  const { register, formState } = useFormContext<T>()
  const error = formState.errors[name]

  return (
    <Field name={name} label={label} hint={hint} required={required}>
      {(fieldProps) => (
        <input
          {...register(name)}
          {...fieldProps}
          type={type}
          placeholder={placeholder}
          disabled={disabled}
          autoComplete={autoComplete}
          className={`form-input${error ? ' error' : ''}`}
        />
      )}
    </Field>
  )
}

// ─────────────────────────────────────────────
// TextareaField
// ─────────────────────────────────────────────

type TextareaFieldProps<T extends FieldValues> = {
  name: Path<T>
  label?: string
  hint?: string
  required?: boolean
  placeholder?: string
  rows?: number
  disabled?: boolean
}

export function TextareaField<T extends FieldValues>({
  name,
  label,
  hint,
  required,
  placeholder,
  rows = 4,
  disabled,
}: TextareaFieldProps<T>) {
  const { register, formState } = useFormContext<T>()
  const error = formState.errors[name]

  return (
    <Field name={name} label={label} hint={hint} required={required}>
      {(fieldProps) => (
        <textarea
          {...register(name)}
          {...fieldProps}
          placeholder={placeholder}
          rows={rows}
          disabled={disabled}
          className={`form-input form-textarea${error ? ' error' : ''}`}
        />
      )}
    </Field>
  )
}

// ─────────────────────────────────────────────
// SelectField
// ─────────────────────────────────────────────

type SelectOption = { label: string; value: string }

type SelectFieldProps<T extends FieldValues> = {
  name: Path<T>
  label?: string
  hint?: string
  required?: boolean
  options: SelectOption[]
  placeholder?: string
  disabled?: boolean
}

export function SelectField<T extends FieldValues>({
  name,
  label,
  hint,
  required,
  options,
  placeholder,
  disabled,
}: SelectFieldProps<T>) {
  const { register, formState } = useFormContext<T>()
  const error = formState.errors[name]

  return (
    <Field name={name} label={label} hint={hint} required={required}>
      {(fieldProps) => (
        <select
          {...register(name)}
          {...fieldProps}
          disabled={disabled}
          className={`form-input${error ? ' error' : ''}`}
          style={{ height: '36px', paddingRight: '28px', appearance: 'none' }}
        >
          {placeholder && (
            <option value="" disabled>
              {placeholder}
            </option>
          )}
          {options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      )}
    </Field>
  )
}

// ─────────────────────────────────────────────
// CheckboxField
// ─────────────────────────────────────────────

type CheckboxFieldProps<T extends FieldValues> = {
  name: Path<T>
  label: string
  hint?: string
  disabled?: boolean
}

export function CheckboxField<T extends FieldValues>({
  name,
  label,
  hint,
  disabled,
}: CheckboxFieldProps<T>) {
  const { register, formState } = useFormContext<T>()
  const error = formState.errors[name]
  const errorMsg = error?.message as string | undefined

  return (
    <div className="form-field">
      <label
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
          cursor: disabled ? 'not-allowed' : 'pointer',
        }}
      >
        <input
          {...register(name)}
          type="checkbox"
          id={name}
          disabled={disabled}
          style={{ accentColor: 'var(--accent)', width: '14px', height: '14px' }}
        />
        <span style={{ fontSize: '13px', color: 'var(--text)' }}>{label}</span>
      </label>
      {hint && !errorMsg && <p className="form-hint">{hint}</p>}
      {errorMsg && (
        <p className="form-error" role="alert">
          <Icon name="alert-circle" size={12} />
          {errorMsg}
        </p>
      )}
    </div>
  )
}

// ─────────────────────────────────────────────
// SubmitButton — aware of form state
// ─────────────────────────────────────────────

type SubmitButtonProps = {
  label?: string
  loadingLabel?: string
  className?: string
  disabled?: boolean
}

export function SubmitButton({
  label = 'Save',
  loadingLabel = 'Saving…',
  className = 'btn btn-primary',
  disabled,
}: SubmitButtonProps) {
  const { formState } = useFormContext()
  const isSubmitting = formState.isSubmitting

  return (
    <button
      type="submit"
      disabled={disabled || isSubmitting}
      className={className}
    >
      {isSubmitting && <span className="spinner spinner-sm" />}
      {isSubmitting ? loadingLabel : label}
    </button>
  )
}

// ─────────────────────────────────────────────
// FormRow — horizontal layout for fields
// ─────────────────────────────────────────────

export function FormRow({ children, columns = 2 }: { children: ReactNode; columns?: number }) {
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: `repeat(${columns}, 1fr)`,
        gap: '12px',
      }}
    >
      {children}
    </div>
  )
}

// ─────────────────────────────────────────────
// FormSection — grouped fields with heading
// ─────────────────────────────────────────────

export function FormSection({
  title,
  description,
  children,
}: {
  title?: string
  description?: string
  children: ReactNode
}) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
      {(title || description) && (
        <div style={{ paddingBottom: '12px', borderBottom: '1px solid var(--border)' }}>
          {title && (
            <p style={{ fontWeight: 600, fontSize: '13px', color: 'var(--text)' }}>{title}</p>
          )}
          {description && (
            <p style={{ fontSize: '12px', color: 'var(--text-subtle)', marginTop: '3px' }}>
              {description}
            </p>
          )}
        </div>
      )}
      {children}
    </div>
  )
}

// Re-export react-hook-form helpers for consumers
export { useFormContext, Controller }
