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
    desc: 'Phân tích file PO gốc (PDF/XLSX/TXT) từ 10 hệ thống MT, tách đúng mã hàng, số lượng, giá theo từng nhà bán lẻ.',
  },
  {
    icon: <FaScaleBalanced />,
    title: 'Đối soát giá & khuyến mãi',
    desc: 'So khớp giá trên PO với giá hệ thống và chương trình khuyến mãi đang áp dụng, lấy trực tiếp từ Google Sheets.',
  },
  {
    icon: <FaTags />,
    title: 'Đánh dấu sai giá',
    desc: 'Mã sai giá được flag rõ ràng trên bảng kết quả, cho chọn áp giá PO hoặc giá hệ thống chỉ với một cú nhấp.',
  },
  {
    icon: <FaFileExcel />,
    title: 'Xuất file chuẩn AMIS',
    desc: 'Ghi thẳng vào dondathang.xlsx theo đúng khuôn cột hệ thống kế toán đang dùng, không cần nhập tay lại.',
  },
  {
    icon: <FaCloudArrowUp />,
    title: 'Lưu trữ chứng từ',
    desc: 'Tự động tải file PO gốc lên Google Drive, đặt tên theo PO/ngày đặt để tra cứu lại khi cần.',
  },
  {
    icon: <FaPaperPlane />,
    title: 'Thông báo Zalo tự động',
    desc: 'Soạn và gửi tin xác nhận đơn hàng qua Zalo cho từng nhóm khách, xem trước nội dung trước khi gửi.',
  },
]

export function InfoTab() {
  return (
    <div className="h-full overflow-auto pr-1">
      <div className="mx-auto flex max-w-4xl flex-col gap-4 pb-2">
        {/* Hero */}
        <section className="animate-rise flex flex-col items-center rounded-2xl border border-border bg-panel px-8 py-9 text-center">
          <AnimatedBlueLogo
            active
            className="h-14 w-auto aspect-[627/332] drop-shadow-[0_0_18px_rgba(40,197,242,0.35)]"
          />
          <h1 className="mt-5 font-mono text-xl font-extrabold tracking-wide text-ink sm:text-2xl">
            BLUE HÀ THÀNH <span className="text-accent">·</span> ORDER SYSTEM
          </h1>
          <p className="mt-1.5 font-mono text-[11px] font-semibold tracking-[0.3em] text-muted">
            PHIÊN BẢN 3.0 — PRODUCTION
          </p>
          <p className="mx-auto mt-4 max-w-xl text-sm leading-relaxed text-ink/80">
            Hệ thống tự động hoá xử lý đơn hàng cho các nhà bán lẻ hiện đại (MT) — từ đọc file PO gốc, đối soát
            giá và khuyến mãi, tới xuất chứng từ và thông báo khách hàng, không cần thao tác thủ công.
          </p>
        </section>

        {/* Stats */}
        <div className="animate-rise grid grid-cols-2 gap-3 [animation-delay:60ms] sm:grid-cols-4">
          <StatCard value="9" label="Hệ thống MT hỗ trợ" />
          <StatCard value="3" label="Định dạng đầu vào" />
          <StatCard value="100%" label="Tự động đối soát giá" />
          <StatCard value="0" label="Thao tác nhập tay" />
        </div>

        {/* Core features */}
        <section className="animate-rise rounded-2xl border border-border bg-panel p-5 [animation-delay:110ms]">
          <SectionHeader index="01" title="Chức năng chính" />
          <div className="grid grid-cols-1 gap-3 pt-1 sm:grid-cols-2 lg:grid-cols-3">
            {CORE_FEATURES.map((f) => (
              <div key={f.title} className="rounded-xl border border-border/70 bg-bg/60 p-3.5">
                <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-accent/10 text-accent">
                  {f.icon}
                </div>
                <p className="mt-2.5 text-[13px] font-bold text-ink">{f.title}</p>
                <p className="mt-1 text-xs leading-relaxed text-muted">{f.desc}</p>
              </div>
            ))}
          </div>
        </section>

        {/* Supported systems + tech stack */}
        <div className="animate-rise grid grid-cols-1 gap-4 [animation-delay:160ms] lg:grid-cols-2">
          <section className="rounded-2xl border border-border bg-panel p-5">
            <SectionHeader index="02" title="Hệ thống hỗ trợ" />
            <div className="flex flex-wrap gap-2 pt-1">
              {SUPPORTED_SYSTEMS.map((s) => (
                <span
                  key={s}
                  className="rounded-full border border-border bg-bg/60 px-3 py-1.5 font-mono text-xs font-semibold text-ink"
                >
                  {s}
                </span>
              ))}
            </div>
          </section>
          <section className="rounded-2xl border border-border bg-panel p-5">
            <SectionHeader index="03" title="Vận hành & bảo mật" />
            <ul className="space-y-2.5 pt-1 text-xs text-ink">
              <li className="flex items-center gap-2.5">
                <FaLock className="shrink-0 text-brandPurple" /> Kiểm tra tình trạng cấp phép theo thời gian thực,
                tự khoá khi hết hạn sử dụng.
              </li>
              <li className="flex items-center gap-2.5">
                <FaGear className="shrink-0 text-brandPurple" /> Cấu hình Google Sheets / Zalo ngay trong app, áp
                dụng ngay khi lưu, không cần khởi động lại.
              </li>
              <li className="flex items-center gap-2.5">
                <FaCloudArrowUp className="shrink-0 text-brandPurple" /> Dữ liệu giá, khuyến mãi và danh mục khách
                hàng lấy trực tiếp từ Google Sheets — cập nhật một nơi, áp dụng mọi máy.
              </li>
            </ul>
          </section>
        </div>

        {/* Author / contact */}
        <section className="animate-rise rounded-2xl border border-border bg-panel p-6 [animation-delay:210ms]">
          <SectionHeader index="04" title="Phát triển bởi" />
          <div className="flex flex-col items-center gap-6 pt-1 sm:flex-row sm:items-start">
            <div className="flex-1">
              <p className="text-base font-bold text-ink">Huỳnh Đạt Thành</p>
              <p className="text-xs font-semibold uppercase tracking-wider text-accent">Tác giả &amp; bảo trì hệ thống</p>
              <div className="mt-3 space-y-1.5 text-sm text-ink/80">
                <p className="flex items-center gap-2">
                  <FaPhone className="text-muted" size={12} /> 0947.940.391
                </p>
                <p className="flex items-center gap-2">
                  <FaEnvelope className="text-muted" size={12} /> byun.huynh@gmail.com
                </p>
              </div>
              <p className="mt-4 max-w-md text-xs leading-relaxed text-muted">
                Có góp ý hoặc phát sinh lỗi trong quá trình sử dụng, vui lòng liên hệ trực tiếp qua số điện thoại
                hoặc email trên để được hỗ trợ.
              </p>
            </div>
            <img
              src="/qr.jpg"
              alt="QR liên hệ"
              className="no-drag h-32 w-32 shrink-0 rounded-xl border border-border object-cover"
              draggable={false}
            />
          </div>
        </section>
      </div>
    </div>
  )
}

function StatCard({ value, label }: { value: string; label: string }) {
  return (
    <div className="rounded-xl border border-border bg-panel px-4 py-3.5 text-center">
      <p className="font-mono text-xl font-extrabold text-accent">{value}</p>
      <p className="mt-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted">{label}</p>
    </div>
  )
}
