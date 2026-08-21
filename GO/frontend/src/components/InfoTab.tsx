export function InfoTab() {
  return (
    <div className="mx-auto flex h-full max-w-2xl flex-col items-center justify-center gap-6 overflow-auto text-center">
      <div className="animate-rise rounded-2xl border border-border bg-panel p-8">
        <img
          src="/logo.svg"
          alt="Blue Hà Thành"
          className="no-drag mx-auto h-10 w-auto drop-shadow-[0_0_18px_rgba(40,197,242,0.35)]"
          draggable={false}
        />
        <h2 className="mt-5 font-mono text-xl font-bold tracking-wide text-accent">
          AUTOMATED ORDER PROCESSING SYSTEM
        </h2>
        <div className="mt-5 space-y-2 text-left text-sm text-ink">
          <p className="text-xs font-bold uppercase tracking-wider text-brandPurple">Chức năng chính</p>
          <ul className="list-disc space-y-1 pl-5 marker:text-accent">
            <li>Phân tích đơn hàng PDF/XLSX/TXT từ hệ thống MT (BigC, Lotte, Satra...)</li>
            <li>Tự động đối soát giá bán và chương trình khuyến mãi.</li>
            <li>Xuất dữ liệu chuẩn hóa phục vụ kế toán.</li>
          </ul>
          <p className="pt-3">
            <span className="text-muted">Tác giả:</span> <span className="font-semibold">HUYNH DAT THANH</span>
          </p>
          <p>
            <span className="text-muted">Liên hệ:</span> 0947.940.391 · byun.huynh@gmail.com
          </p>
        </div>
        <img
          src="/qr.jpg"
          alt="QR liên hệ"
          className="no-drag mx-auto mt-6 h-40 w-40 rounded-lg border border-border object-cover"
          draggable={false}
        />
      </div>
    </div>
  )
}
