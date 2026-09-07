import type { POContentGroup } from '../components/OrderContentModal'

// Nhóm Zalo nào sẽ nhận tin nào — phần dữ liệu thuần của bản xem trước,
// tách khỏi component để test được mà không cần dựng DOM.
//
// Việc CHỌN nhóm nằm hoàn toàn ở Go (zalosend.ResolveTarget): key ghép
// từ miền + phân khúc + hệ thống rồi tra Cài đặt > Zalo. Cố ý KHÔNG dựng
// lại logic đó ở đây - một bản sao TypeScript sẽ trôi lệch khỏi bản Go
// theo thời gian, và hậu quả là bản xem trước hiện một nhóm còn tin bay
// tới nhóm khác. Frontend chỉ dựng câu hỏi và hiển thị câu trả lời.

// Khớp main.ZaloTargetPreview (app.go). Không import từ wailsjs/models
// để file này còn test được bằng node --test, vốn không có window.go.
export interface ZaloTargetPreview {
  po: string
  configured: boolean
  groupName: string
  key: string
  suggestedKey: string
  candidateKeys: string[]
}

export interface ZaloTargetQuery {
  po: string
  system: string
  customerCode: string
}

// zaloTargetQueriesFor dựng câu hỏi cho từng nhóm tin, lấy system và mã
// khách hàng từ dòng ĐẦU của nhóm — ĐÚNG như ControlPanel.handleSendZalo
// dựng ZaloJob. Hai chỗ phải đọc cùng một dòng: BigC gộp nhiều dòng cửa
// hàng vào một tin, và chỉ dòng đầu mới quyết định nhóm nhận.
export function zaloTargetQueriesFor(groups: POContentGroup[]): ZaloTargetQuery[] {
  return groups.map((g) => ({
    po: g.po,
    system: g.rows[0]?.system ?? '',
    customerCode: g.rows[0]?.maKhachHang ?? '',
  }))
}

// targetByPO đánh chỉ mục kết quả theo po để component tra được O(1) khi
// render từng bong bóng tin. po ở đây là KHOÁ GỘP NHÓM (có thể là hash
// sourceId của JIT hay "tmdt|<shop>"), không nhất thiết là số PO thật —
// chỉ cần khớp đúng chuỗi đã gửi đi trong query.
export function targetByPO(previews: ZaloTargetPreview[]): Record<string, ZaloTargetPreview | undefined> {
  const index: Record<string, ZaloTargetPreview | undefined> = {}
  for (const preview of previews) {
    index[preview.po] = preview
  }
  return index
}
