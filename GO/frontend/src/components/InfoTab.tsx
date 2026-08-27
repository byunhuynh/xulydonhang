import {
  FaFilePdf,
  FaScaleBalanced,
  FaTags,
  FaFileExcel,
  FaCloudArrowUp,
  FaPaperPlane,
  FaLock,
  FaGear,
  FaPhone,
  FaEnvelope,
} from 'react-icons/fa6'
import AnimatedBlueLogo from './AnimatedBlueLogo'
import { SectionHeader } from './SectionHeader'

const SUPPORTED_SYSTEMS = ['Co.op Mart', 'BigC / GO!', 'Lotte Mart', 'Satra', 'Emart', 'Kingfood Mart', 'WinMart', 'FujiMart', 'JMart', 'Maxidi']

const CORE_FEATURES: { icon: React.ReactNode; title: string; desc: string }[] = [
  {
    icon: <FaFilePdf />,
    title: 'Đọc đơn hàng tự động',
    desc: 'Phân tích PO gốc (PDF/XLSX/TXT) từ 10 hệ thống MT, tách đúng mã hàng/SL/giá.',
  },
  {
    icon: <FaScaleBalanced />,
    title: 'Đối soát giá & khuyến mãi',
    desc: 'So khớp giá PO với giá hệ thống và khuyến mãi, lấy trực tiếp từ Google Sheets.',
  },
  {
    icon: <FaTags />,
    title: 'Đánh dấu sai giá',
    desc: 'Flag rõ mã sai giá, chọn áp giá PO hoặc giá hệ thống chỉ với một cú nhấp.',
  },
  {
    icon: <FaFileExcel />,
    title: 'Xuất file chuẩn AMIS',
    desc: 'Ghi thẳng vào dondathang.xlsx đúng khuôn cột kế toán, không cần nhập tay lại.',
  },
  {
    icon: <FaCloudArrowUp />,
    title: 'Lưu trữ chứng từ',
    desc: 'Tự động tải PO gốc lên Google Drive, đặt tên theo PO/ngày để tra cứu.',
  },
  {
    icon: <FaPaperPlane />,
    title: 'Thông báo Zalo tự động',
    desc: 'Soạn và gửi tin xác nhận đơn qua Zalo cho từng nhóm khách, xem trước trước khi gửi.',
  },
]

export function InfoTab() {
  return (
    <div className="mx-auto flex h-full max-w-5xl flex-col gap-3 overflow-auto">
      {/* Hero + stats, gộp 1 hàng */}
      <section className="animate-rise flex shrink-0 items-center gap-5 rounded-2xl border border-border bg-panel px-6 py-4">
        <AnimatedBlueLogo
          active
          className="h-11 w-auto aspect-[627/332] shrink-0 drop-shadow-[0_0_14px_rgba(40,197,242,0.35)]"
        />
        <div className="min-w-0 flex-1">
          <h1 className="font-mono text-base font-extrabold tracking-wide text-ink sm:text-lg">
            BLUE HÀ THÀNH <span className="text-accent">·</span> ORDER SYSTEM
            <span className="ml-2 font-mono text-[10px] font-semibold tracking-[0.25em] text-muted">V3.0</span>
          </h1>
          <p className="mt-0.5 truncate text-xs leading-relaxed text-ink/70">
            Tự động hoá xử lý đơn hàng MT — đọc PO, đối soát giá/khuyến mãi, xuất chứng từ, thông báo khách hàng.
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <StatPill value="9" label="Hệ thống" />
          <StatPill value="3" label="Định dạng" />
          <StatPill value="100%" label="Đối soát" />
        </div>
      </section>

      {/* Core features */}
      <section className="animate-rise shrink-0 rounded-2xl border border-border bg-panel p-4 [animation-delay:60ms]">
        <SectionHeader index="01" title="Chức năng chính" />
        <div className="grid grid-cols-3 gap-2.5 pt-1">
          {CORE_FEATURES.map((f) => (
            <div key={f.title} className="rounded-xl border border-border/70 bg-bg/60 p-2.5">
              <div className="flex items-center gap-2">
                <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-accent/10 text-[13px] text-accent">
                  {f.icon}
                </div>
                <p className="text-[12.5px] font-bold leading-tight text-ink">{f.title}</p>
              </div>
              <p className="mt-1.5 text-[11px] leading-snug text-muted">{f.desc}</p>
            </div>
          ))}
        </div>
      </section>

      {/* Supported systems + ops/security + author, gộp 1 hàng 3 cột */}
      <div className="animate-rise grid min-h-0 flex-1 grid-cols-3 gap-3 [animation-delay:110ms]">
        <section className="rounded-2xl border border-border bg-panel p-4">
          <SectionHeader index="02" title="Hệ thống hỗ trợ" />
          <div className="flex flex-wrap gap-1.5 pt-1">
            {SUPPORTED_SYSTEMS.map((s) => (
              <span
                key={s}
                className="rounded-full border border-border bg-bg/60 px-2.5 py-1 font-mono text-[10.5px] font-semibold text-ink"
              >
                {s}
              </span>
            ))}
          </div>
        </section>
        <section className="rounded-2xl border border-border bg-panel p-4">
          <SectionHeader index="03" title="Vận hành & bảo mật" />
          <ul className="space-y-2 pt-1 text-[11.5px] leading-snug text-ink">
            <li className="flex items-start gap-2">
              <FaLock className="mt-0.5 shrink-0 text-brandPurple" size={11} /> Kiểm tra cấp phép theo thời gian
              thực, tự khoá khi hết hạn.
            </li>
            <li className="flex items-start gap-2">
              <FaGear className="mt-0.5 shrink-0 text-brandPurple" size={11} /> Cấu hình Sheets/Zalo ngay trong
              app, áp dụng ngay khi lưu.
            </li>
            <li className="flex items-start gap-2">
              <FaCloudArrowUp className="mt-0.5 shrink-0 text-brandPurple" size={11} /> Giá, khuyến mãi, danh mục
              khách hàng lấy trực tiếp từ Google Sheets.
            </li>
          </ul>
        </section>
        <section className="rounded-2xl border border-border bg-panel p-4">
          <SectionHeader index="04" title="Phát triển bởi" />
          <div className="flex items-center gap-3 pt-1">
            <img
              src="/qr.jpg"
              alt="QR liên hệ"
              className="no-drag h-14 w-14 shrink-0 rounded-lg border border-border object-cover"
              draggable={false}
            />
            <div className="min-w-0">
              <p className="text-sm font-bold text-ink">Huỳnh Đạt Thành</p>
              <p className="text-[10px] font-semibold uppercase tracking-wider text-accent">Tác giả &amp; bảo trì</p>
              <p className="mt-1.5 flex items-center gap-1.5 text-[11px] text-ink/80">
                <FaPhone className="shrink-0 text-muted" size={10} /> 0947.940.391
              </p>
              <p className="mt-0.5 flex items-center gap-1.5 truncate text-[11px] text-ink/80">
                <FaEnvelope className="shrink-0 text-muted" size={10} /> byun.huynh@gmail.com
              </p>
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}

function StatPill({ value, label }: { value: string; label: string }) {
  return (
    <div className="rounded-xl border border-border bg-bg/60 px-3 py-1.5 text-center">
      <p className="font-mono text-sm font-extrabold text-accent">{value}</p>
      <p className="text-[8.5px] font-semibold uppercase tracking-wider text-muted">{label}</p>
    </div>
  )
}
