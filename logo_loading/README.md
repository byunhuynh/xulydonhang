# Logo "Blue" + màn hình loading

Chép từ `GO/frontend/src` của app Xử lý đơn hàng, để mang sang app khác.
Các file nằm phẳng trong một thư mục, import trỏ lẫn nhau bằng `./...`,
nên chỉ cần chép NGUYÊN thư mục này vào dự án mới (ví dụ `src/logo_loading/`).

## Các file

| File | Vai trò |
|---|---|
| `blueLogo.ts` | Dữ liệu path SVG của logo (sóng cyan, khối tím, chữ "Blue", đường chạy). Sinh từ `blue_logo_vector.svg` — không sửa tay. |
| `snakePath.ts` | Thuật toán uốn dải sóng bò quanh viền kiểu con rắn. Không phụ thuộc thư viện nào. |
| `AnimatedBlueLogo.tsx` + `.css` | Logo dùng chung. `active={true}` thì dải sóng bò vòng, `false` thì đứng yên. |
| `StartupLogoLoader.tsx` + `.css` | Màn chờ: logo hiện dần, "thở" nhẹ, kèm chữ trạng thái và ba dấu chấm nhảy. |
| `blue_logo_vector.svg` | File logo gốc (dùng làm logo tĩnh / favicon nếu cần). |
| `snakePath.test.ts` | Test cho `snakePath.ts` (không bắt buộc chép). |

## Yêu cầu

- React 18+ (app gốc dùng React 19), TypeScript, bundler hỗ trợ `import './x.css'` (Vite, Next…).
- Cài GSAP:

  ```bash
  npm install gsap@^3.15.0 @gsap/react@^2.1.2
  ```

Không cần Tailwind — CSS của logo là CSS thuần.

## Cách dùng

```tsx
import AnimatedBlueLogo from './logo_loading/AnimatedBlueLogo'
import StartupLogoLoader from './logo_loading/StartupLogoLoader'

// Logo trên header, chỉ chạy khi đang bận:
<AnimatedBlueLogo active={isBusy} className="my-logo" />

// Màn hình loading:
{loading && <StartupLogoLoader label="Đang tải dữ liệu" />}
```

Lưu ý:

- `AnimatedBlueLogo` KHÔNG tự đặt kích thước — đặt qua `className`, ví dụ
  `.my-logo { width: 160px; aspect-ratio: 627 / 332; }`.
- Dải sóng thò ra ngoài khung logo ~11% chiều cao lúc bò lên đỉnh; chừa
  khoảng trống xung quanh, đừng để cha có `overflow: hidden` sát logo.
- Chữ trạng thái trong loader màu `#f1f0fb` (sáng) — thiết kế cho nền tối.
  Nền sáng thì sửa `.startup-logo__label` trong `StartupLogoLoader.css`.
- Tôn trọng `prefers-reduced-motion`: người dùng tắt chuyển động sẽ thấy logo tĩnh.

## Chỉnh nhanh

- Tốc độ một vòng: `LAP_DURATION` (giây) trong `AnimatedBlueLogo.tsx`.
- Nghỉ giữa các vòng: `HOLD_SECONDS`.
- Màu: `#28C5F2` (cyan), `#5453A1` (tím), `#FFFFFF` (chữ) trong `AnimatedBlueLogo.tsx`.

## Chạy test (tuỳ chọn)

Test dùng `node:test`, cần Node 22.6+ và `"type": "module"` trong package.json:

```bash
node --experimental-strip-types --test logo_loading/snakePath.test.ts
```
