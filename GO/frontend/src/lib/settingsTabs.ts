// Danh sách tab của popup Cài đặt, tách khỏi SettingsModal.tsx để
// appStore tham chiếu được kiểu này mà không kéo cả component (và cùng
// với nó là mọi editor con) vào đồ thị import của store.
export type SettingsTab = 'gid' | 'zalo' | 'reminder' | 'haravan' | 'misa' | 'misaRouting' | 'warehouse'
