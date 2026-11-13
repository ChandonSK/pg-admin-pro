import { useEffect, useState, type FormEvent, type ReactNode } from 'react';
import { AlertTriangle, Loader2 } from 'lucide-react';
import { Modal } from './Modal';

interface ConfirmModalProps {
  open: boolean;
  onClose: () => void;
  title: string;
  subtitle?: string;
  message: ReactNode;
  confirmTextMatch?: string;
  confirmLabel?: string;
  loading?: boolean;
  onConfirm: () => Promise<void> | void;
}

export function ConfirmModal({
  open,
  onClose,
  title,
  subtitle,
  message,
  confirmTextMatch,
  confirmLabel = 'Delete Permanently',
  loading = false,
  onConfirm,
}: ConfirmModalProps) {
  const [typed, setTyped] = useState('');

  useEffect(() => {
    if (open) {
      setTyped('');
    }
  }, [open, confirmTextMatch]);

  const canConfirm = !confirmTextMatch || typed === confirmTextMatch;

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!canConfirm || loading) return;
    await onConfirm();
  };

  return (
    <Modal open={open} onClose={onClose} title={title} subtitle={subtitle}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="flex items-start gap-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3.5 text-xs text-red-700 dark:text-red-200">
          <AlertTriangle className="w-4 h-4 text-red-500 shrink-0 mt-0.5" />
          <div className="leading-relaxed">{message}</div>
        </div>

        {confirmTextMatch && (
          <div className="space-y-1.5">
            <label className="block text-xs text-slate-600 dark:text-slate-300">
              Type{' '}
              <code className="px-1.5 py-0.5 rounded bg-red-500/10 font-mono font-bold text-red-500">
                {confirmTextMatch}
              </code>{' '}
              to confirm:
            </label>
            <input
              type="text"
              autoFocus
              required
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={confirmTextMatch}
              className="w-full rounded-xl border border-red-500/40 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm font-mono text-slate-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-red-500"
            />
          </div>
        )}

        <div className="flex justify-end gap-2 pt-1">
          <button
            type="button"
            disabled={loading}
            onClick={onClose}
            className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs font-medium hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={loading || !canConfirm}
            className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-red-600 hover:bg-red-500 disabled:opacity-40 text-white text-xs font-semibold shadow-sm transition-colors"
          >
            {loading && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
            {confirmLabel}
          </button>
        </div>
      </form>
    </Modal>
  );
}
