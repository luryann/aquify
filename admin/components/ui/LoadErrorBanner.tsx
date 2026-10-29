import { Icon } from '@/components/ui/icons'
import './LoadErrorBanner.inline.css'

/** Warning banner shown when a page's data failed to load, with a retry action. */
export function LoadErrorBanner({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div role="alert" className="load-error">
      <div className="load-error-body">
        <Icon name="alert-circle" size={15} />
        <p>{message}</p>
      </div>
      {onRetry ? (
        <button type="button" className="load-error-retry" onClick={onRetry}>
          Retry
        </button>
      ) : null}
    </div>
  )
}
