/**
 * Uốn một hình SVG kín chạy dọc theo một đường kín khác mà vẫn giữ
 * nguyên khoảng cách vuông góc của từng điểm tới đường đó ("warp to
 * path", kiểu con rắn bò men theo mép).
 *
 * Dùng cho màn loading: dải sóng cyan của logo bò vòng quanh viền khối
 * tím, lúc nào cũng cách viền đúng bằng khoảng cách nó có trong logo
 * gốc.
 *
 * Ý tưởng: mỗi điểm P của hình được đổi sang toạ độ (s, v) so với
 * đường chạy - s là quãng đường dọc theo đường, v là khoảng cách vuông
 * góc (dương ra phía ngoài). Muốn hình dịch đi bao nhiêu thì chỉ cần
 * cộng thêm vào s rồi dựng lại: Q = điểm(s) + pháp_tuyến(s) * v. Vì
 * phép đổi toạ độ là phép chiếu vuông góc nên với shift = 0 ta dựng lại
 * ĐÚNG hình ban đầu, và vì đường chạy kín nên shift = chu vi cũng cho
 * lại đúng hình ban đầu -> chạy hết một vòng là về chỗ cũ khít khịt.
 *
 * Đường bao trong file SVG gốc là nét vẽ dò theo pixel: toàn đoạn thẳng
 * 1-3 đơn vị, bậc thang ngang-dọc xen kẽ. Pháp tuyến tính thẳng trên
 * đó sẽ nhảy 90 độ liên tục, nên buildTrack lấy mẫu đều -> làm mượt ->
 * lấy mẫu đều lần nữa trước khi tính pháp tuyến.
 *
 * Một điểm nữa quan trọng không kém: KHÔNG chạy trực tiếp trên đường
 * bao, mà chạy trên đường ĐI GIỮA dải (buildRibbon). Lý do là hệ số
 * giãn/nén cục bộ của phép trải bằng 1 + v * độ_cong: đường bao khối
 * tím có hai mũi nhọn bán kính chỉ 4-6.5 đơn vị, trong khi dải sóng dày
 * 30 và nằm cách viền tới 37.5 đơn vị, nên qua mũi nhọn hệ số vọt lên
 * 3.5 lần - dải bị kéo toạc ra. Lấy đường giữa dải làm đường chạy thì
 * bán kính chỗ đó thành ~25 còn v chỉ còn +-15, hệ số về khoảng 0.7-1.3
 * (uốn cong bình thường của một dải mềm) mà hình lúc nghỉ vẫn trùng
 * khít bản gốc.
 */

export type Pt = { x: number; y: number }

/** Vị trí của một điểm so với đường chạy: s = quãng đường dọc đường, v = lệch vuông góc. */
export type Anchor = { s: number; v: number }

export type Track = {
  /** Các mẫu cách đều nhau đúng `step` đơn vị. */
  points: Pt[]
  /** Pháp tuyến đơn vị tại từng mẫu. */
  normals: Pt[]
  step: number
  perimeter: number
}

export type TrackOptions = {
  /** Khoảng cách giữa hai mẫu liên tiếp (đơn vị của viewBox). */
  step?: number
  /** Bán kính cửa sổ trung bình trượt khi làm mượt, tính bằng số mẫu. */
  smoothRadius?: number
  /** Tiếp tuyến lấy theo sai phân trung tâm cách nhau bấy nhiêu mẫu. */
  tangentSpan?: number
}

const DEFAULTS = { step: 1, smoothRadius: 16, tangentSpan: 4 } as const

/** Đọc mọi cặp toạ độ "x,y" trong path data (chỉ dùng được cho path toàn M/L/Z). */
export function parsePathPoints(d: string): Pt[] {
  const points: Pt[] = []
  const re = /(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)/g
  let match: RegExpExecArray | null
  while ((match = re.exec(d))) {
    points.push({ x: Number(match[1]), y: Number(match[2]) })
  }
  return points
}

const distance = (a: Pt, b: Pt): number => Math.hypot(b.x - a.x, b.y - a.y)

/** Chu vi của đa giác kín (đã tính cả cạnh nối điểm cuối về điểm đầu). */
export function closedLength(points: Pt[]): number {
  let total = 0
  for (let i = 0; i < points.length; i += 1) {
    total += distance(points[i], points[(i + 1) % points.length])
  }
  return total
}

/** Lấy mẫu lại đa giác kín thành các điểm cách đều nhau. */
export function resampleClosed(points: Pt[], step: number): Pt[] {
  const perimeter = closedLength(points)
  const count = Math.max(3, Math.round(perimeter / step))
  const spacing = perimeter / count
  const out: Pt[] = []

  let segment = 0
  let consumed = 0 // đã đi được bao nhiêu trong cạnh hiện tại
  let segmentLength = distance(points[0], points[1 % points.length])

  for (let i = 0; i < count; i += 1) {
    let remaining = i === 0 ? 0 : spacing
    while (remaining > 0) {
      const left = segmentLength - consumed
      if (remaining < left || segment === points.length - 1) {
        consumed += remaining
        remaining = 0
      } else {
        remaining -= left
        segment += 1
        consumed = 0
        segmentLength = distance(points[segment], points[(segment + 1) % points.length])
      }
    }
    const a = points[segment]
    const b = points[(segment + 1) % points.length]
    const t = segmentLength === 0 ? 0 : consumed / segmentLength
    out.push({ x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t })
  }
  return out
}

/** Trung bình trượt vòng tròn - dập bậc thang của nét vẽ dò pixel. */
export function smoothClosed(points: Pt[], radius: number): Pt[] {
  if (radius <= 0) return points.slice()
  const n = points.length
  const out: Pt[] = []
  for (let i = 0; i < n; i += 1) {
    let sx = 0
    let sy = 0
    for (let k = -radius; k <= radius; k += 1) {
      const p = points[(i + k + n * (radius + 1)) % n]
      sx += p.x
      sy += p.y
    }
    const count = radius * 2 + 1
    out.push({ x: sx / count, y: sy / count })
  }
  return out
}

/** Dựng đường chạy: lấy mẫu đều -> làm mượt -> lấy mẫu đều -> tính pháp tuyến. */
export function buildTrack(raw: Pt[], options: TrackOptions = {}): Track {
  const step = options.step ?? DEFAULTS.step
  const smoothRadius = options.smoothRadius ?? DEFAULTS.smoothRadius
  const tangentSpan = options.tangentSpan ?? DEFAULTS.tangentSpan

  const points = resampleClosed(smoothClosed(resampleClosed(raw, step), smoothRadius), step)
  const n = points.length
  const perimeter = closedLength(points)

  const normals: Pt[] = []
  for (let i = 0; i < n; i += 1) {
    const ahead = points[(i + tangentSpan) % n]
    const behind = points[(i - tangentSpan + n) % n]
    const tx = ahead.x - behind.x
    const ty = ahead.y - behind.y
    const len = Math.hypot(tx, ty) || 1
    // Pháp tuyến = tiếp tuyến quay -90 độ.
    normals.push({ x: ty / len, y: -tx / len })
  }

  return { points, normals, step: perimeter / n, perimeter }
}

/** Vị trí trên đường chạy tại quãng đường s (nội suy tuyến tính giữa 2 mẫu). */
export function pointAt(track: Track, s: number): Pt {
  const n = track.points.length
  const raw = s / track.step
  const wrapped = ((raw % n) + n) % n
  const i = Math.floor(wrapped)
  const t = wrapped - i
  const a = track.points[i]
  const b = track.points[(i + 1) % n]
  return { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t }
}

/** Pháp tuyến đơn vị tại quãng đường s. */
export function normalAt(track: Track, s: number): Pt {
  const n = track.normals.length
  const raw = s / track.step
  const wrapped = ((raw % n) + n) % n
  const i = Math.floor(wrapped)
  const t = wrapped - i
  const a = track.normals[i]
  const b = track.normals[(i + 1) % n]
  const x = a.x + (b.x - a.x) * t
  const y = a.y + (b.y - a.y) * t
  const len = Math.hypot(x, y) || 1
  return { x: x / len, y: y / len }
}

/** Chiếu từng điểm của hình lên đường chạy để lấy toạ độ (s, v). */
export function projectOntoTrack(points: Pt[], track: Track): Anchor[] {
  const n = track.points.length
  return points.map((p) => {
    let best = 0
    let bestDistance = Infinity
    for (let i = 0; i < n; i += 1) {
      const dx = p.x - track.points[i].x
      const dy = p.y - track.points[i].y
      const d = dx * dx + dy * dy
      if (d < bestDistance) {
        bestDistance = d
        best = i
      }
    }
    const base = track.points[best]
    const normal = track.normals[best]
    // Tiếp tuyến = pháp tuyến quay +90 độ (ngược chiều lúc dựng normals).
    const tangent = { x: -normal.y, y: normal.x }
    const dx = p.x - base.x
    const dy = p.y - base.y
    return {
      s: best * track.step + dx * tangent.x + dy * tangent.y,
      v: dx * normal.x + dy * normal.y,
    }
  })
}

const round = (value: number): string => {
  const r = Math.round(value * 100) / 100
  return Object.is(r, -0) ? '0' : String(r)
}

/** Dựng lại path data của hình sau khi trượt `shift` đơn vị dọc đường chạy. */
export function renderSnakePath(anchors: Anchor[], track: Track, shift: number): string {
  let d = ''
  for (let i = 0; i < anchors.length; i += 1) {
    const { s, v } = anchors[i]
    const base = pointAt(track, s + shift)
    const normal = normalAt(track, s + shift)
    d += `${i === 0 ? 'M' : ' L'}${round(base.x + normal.x * v)},${round(base.y + normal.y * v)}`
  }
  return `${d} Z`
}

/** Khoảng lệch vuông góc lớn nhất - dùng để nới viewBox cho khỏi bị cắt. */
export function maxOffset(anchors: Anchor[]): number {
  return anchors.reduce((max, a) => Math.max(max, Math.abs(a.v)), 0)
}

export type Ribbon = { track: Track; anchors: Anchor[] }

/**
 * Dựng đường chạy cho một dải hình bám theo một đường bao.
 *
 * Chiếu thử dải lên đường bao để biết nó nằm cách viền từ đâu tới đâu,
 * rồi dời đường bao ra đúng khoảng giữa của dải và lấy ĐƯỜNG ĐÓ làm
 * đường chạy. Nhờ vậy v của mọi điểm phân bố đối xứng quanh 0 (thay vì
 * lệch hẳn một bên), và mũi nhọn của đường bao khi dời ra cũng nở thành
 * cung tròn - hai thứ này cùng kéo hệ số giãn/nén 1 + v * độ_cong về
 * sát 1. Xem đầu file để biết vì sao điều đó quan trọng.
 */
export function buildRibbon(shape: Pt[], outline: Pt[], options: TrackOptions = {}): Ribbon {
  const base = buildTrack(outline, options)
  const probe = projectOntoTrack(shape, base)
  const offsets = probe.map((a) => a.v)
  const midline = (Math.min(...offsets) + Math.max(...offsets)) / 2

  const shifted = base.points.map((p, i) => ({
    x: p.x + base.normals[i].x * midline,
    y: p.y + base.normals[i].y * midline,
  }))
  const track = buildTrack(shifted, options)
  return { track, anchors: projectOntoTrack(shape, track) }
}

/** Độ cong có dấu tại từng mẫu của đường chạy. */
export function curvature(track: Track, span = 4): number[] {
  const n = track.points.length
  return track.points.map((_, i) => {
    const a = track.points[(i - span + n) % n]
    const b = track.points[i]
    const c = track.points[(i + span) % n]
    const v1x = b.x - a.x
    const v1y = b.y - a.y
    const v2x = c.x - b.x
    const v2y = c.y - b.y
    return Math.atan2(v1x * v2y - v1y * v2x, v1x * v2x + v1y * v2y) / (span * track.step)
  })
}

/**
 * Hệ số giãn/nén cục bộ tại từng điểm của dải: 1 + v * độ_cong.
 * 1 = giữ nguyên, >1 = bị kéo giãn, <1 = bị nén, <=0 = gấp nếp lên nhau.
 */
export function stretchFactors({ track, anchors }: Ribbon): number[] {
  const ks = curvature(track)
  const n = ks.length
  return anchors.map((a) => 1 + a.v * ks[((Math.round(a.s / track.step) % n) + n) % n])
}
