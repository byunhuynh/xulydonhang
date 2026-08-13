export function InfoTab() {
  return (
    <div className="mx-auto flex h-full max-w-2xl flex-col items-center justify-center gap-6 overflow-auto text-center">
      <div className="rounded-2xl border border-border bg-panel p-8">
        <h2 className="font-mono text-2xl font-semibold text-accent">
          AUTOMATED ORDER PROCESSING SYSTEM
        </h2>
        <div className="mt-4 space-y-2 text-left text-sm text-ink">
          <p className="font-medium text-muted">Chức năng chính:</p>
          <ul className="list-disc space-y-1 pl-5">
            <li>Phân tích đơn hàng PDF/XLSX/TXT từ hệ thống MT (BigC, Lotte, Satra...)</li>
            <li>Tự động đối soát giá bán và chương trình khuyến mãi.</li>
            <li>Xuất dữ liệu chuẩn hóa phục vụ kế toán.</li>
          </ul>
          <p className="pt-2">
            <span className="text-muted">Tác giả:</span> HUYNH DAT THANH
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
