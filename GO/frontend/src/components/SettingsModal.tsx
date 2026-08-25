// GO/frontend/src/components/SettingsModal.tsx
import { useEffect, useRef, useState } from 'react'
import { FaXmark } from 'react-icons/fa6'
import { GetAppSettings, SaveAppSettings } from '../../wailsjs/go/main/App'
import { useAppStore } from '../store/appStore'
import type { AppSettings } from '../types'
import { KeyValueEditor } from './KeyValueEditor'
import { useModalEntrance } from '../lib/useModalEntrance'

type SettingsTab = 'gid' | 'zalo' | 'reminder' | 'haravan'

interface SettingsModalProps {
  onClose: () => void
}

export function SettingsModal({ onClose }: SettingsModalProps) {
  const [tab, setTab] = useState<SettingsTab>('gid')
  const [settings, setSettings] = useState<AppSettings | null>(null)
  const [saved, setSaved] = useState(false)
  const [dupState, setDupState] = useState({ gid: false, zalo: false, reminder: false, haravan: false })
  const appendLog = useAppStore((s) => s.appendLog)
  const backdropRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  useModalEntrance(backdropRef, cardRef, [!!settings])

  useEffect(() => {
    GetAppSettings()
      .then((s) => setSettings(s))
      .catch((err) => appendLog(`❌ Lỗi tải cấu hình: ${String(err)}`))
  }, [appendLog])

  if (!settings) {
    return (
      <div ref={backdropRef} className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
        <div ref={cardRef} className="rounded-xl border border-border bg-panel p-6 text-sm text-muted">
          Đang tải...
        </div>
      </div>
    )
  }

  const hasDuplicates = dupState.gid || dupState.zalo || dupState.reminder || dupState.haravan

  async function handleSave() {
    if (!settings) return
    try {
      await SaveAppSettings(settings)
      setSaved(true)
    } catch (err) {
      appendLog(`❌ Lỗi lưu cấu hình: ${String(err)}`)
    }
  }

  const tabs: { key: SettingsTab; label: string }[] = [
    { key: 'gid', label: 'Google Sheets (GID)' },
    { key: 'zalo', label: 'Zalo' },
    { key: 'reminder', label: 'Nhắc nhở' },
    { key: 'haravan', label: 'Haravan (TMĐT)' },
  ]

  return (
    <div ref={backdropRef} className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
      <div
        ref={cardRef}
        className="flex max-h-[80vh] w-[560px] flex-col rounded-xl border border-border bg-panel p-4"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-sans text-sm font-bold text-ink">Cấu hình app</h2>
          <button type="button" onClick={onClose} className="text-muted hover:text-ink">
            <FaXmark size={16} />
          </button>
        </div>
        <div className="mb-3 flex gap-1 border-b border-border">
          {tabs.map((t) => (
            <button
              key={t.key}
              type="button"
              onClick={() => setTab(t.key)}
              className={`px-3 py-2 font-sans text-xs font-semibold transition-colors ${
                tab === t.key ? 'border-b-2 border-accent text-accent' : 'text-muted hover:text-ink'
              }`}
            >
              {t.label}
            </button>
          ))}
        </div>
        <div className="flex-1 overflow-y-auto">
          {tab === 'gid' && (
            <KeyValueEditor
              entries={settings.gid}
              onChange={(gid) => setSettings({ ...settings, gid })}
              onDuplicateChange={(hasDup) => setDupState((d) => ({ ...d, gid: hasDup }))}
              keyLabel="Hệ thống"
              valueLabel="Gid"
              valueType="number"
            />
          )}
          {tab === 'zalo' && (
            <KeyValueEditor
              entries={settings.zalo}
              onChange={(zalo) => setSettings({ ...settings, zalo })}
              onDuplicateChange={(hasDup) => setDupState((d) => ({ ...d, zalo: hasDup }))}
              keyLabel="Nhóm"
              valueLabel="Tên hiển thị"
              valueType="text"
            />
          )}
          {tab === 'reminder' && (
            <KeyValueEditor
              entries={settings.reminder}
              onChange={(reminder) => setSettings({ ...settings, reminder })}
              onDuplicateChange={(hasDup) => setDupState((d) => ({ ...d, reminder: hasDup }))}
              keyLabel="Nhóm"
              valueLabel="Bật"
              valueType="toggle"
            />
          )}
          {tab === 'haravan' && (
            <KeyValueEditor
              entries={settings.haravan}
              onChange={(haravan) => setSettings({ ...settings, haravan })}
              onDuplicateChange={(hasDup) => setDupState((d) => ({ ...d, haravan: hasDup }))}
              keyLabel="Khoá"
              valueLabel="Giá trị"
              valueType="text"
            />
          )}
        </div>
        <div className="mt-3 flex items-center justify-between border-t border-border pt-3">
          {saved ? (
            <span className="font-sans text-xs text-success">Đã lưu và áp dụng ngay.</span>
          ) : (
            <span />
          )}
          <button
            type="button"
            onClick={handleSave}
            disabled={hasDuplicates}
            className="rounded-lg bg-accent px-4 py-2 font-sans text-xs font-bold text-[#0a1620] transition-opacity disabled:cursor-not-allowed disabled:opacity-40"
          >
            Lưu
          </button>
        </div>
      </div>
    </div>
  )
}
