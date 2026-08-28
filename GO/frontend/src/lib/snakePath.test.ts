import assert from 'node:assert/strict'
import test from 'node:test'

import {
  buildRibbon,
  buildTrack,
  closedLength,
  parsePathPoints,
  projectOntoTrack,
  renderSnakePath,
  stretchFactors,
  type Pt,
} from './snakePath.ts'
import { LOGO_TRACK_D, LOGO_WAVE_D } from '../components/blueLogo.ts'

const logoRibbon = () => buildRibbon(parsePathPoints(LOGO_WAVE_D), parsePathPoints(LOGO_TRACK_D))

const circle = (radius: number, count: number): Pt[] =>
  Array.from({ length: count }, (_, i) => {
    const angle = (i / count) * Math.PI * 2
    return { x: radius * Math.cos(angle), y: radius * Math.sin(angle) }
  })

test('parsePathPoints reads every coordinate pair of an M/L/Z path', () => {
  assert.deepEqual(parsePathPoints('M10,20 L30,40 L-5,6 Z'), [
    { x: 10, y: 20 },
    { x: 30, y: 40 },
    { x: -5, y: 6 },
  ])
})

test('buildTrack keeps the perimeter and produces unit normals', () => {
  const track = buildTrack(circle(100, 240), { step: 1, smoothRadius: 2, tangentSpan: 4 })

  // Làm mượt co đường lại một chút, nhưng không được lệch quá 2%.
  assert.ok(Math.abs(track.perimeter - 2 * Math.PI * 100) < 0.02 * 2 * Math.PI * 100)
  for (const n of track.normals) {
    assert.ok(Math.abs(Math.hypot(n.x, n.y) - 1) < 1e-9)
  }
})

test('projecting then rendering with no shift rebuilds the original shape', () => {
  const { track, anchors } = logoRibbon()
  const wave = parsePathPoints(LOGO_WAVE_D)
  const rebuilt = parsePathPoints(renderSnakePath(anchors, track, 0))

  assert.equal(rebuilt.length, wave.length)
  const worst = Math.max(
    ...wave.map((p, i) => Math.hypot(rebuilt[i].x - p.x, rebuilt[i].y - p.y)),
  )
  // Logo rộng 627 đơn vị, nên 0.5 đơn vị là dưới một phần năm pixel.
  assert.ok(worst < 0.5, `lệch tối đa ${worst}`)
})

test('a full lap lands back on the starting shape', () => {
  const { track, anchors } = logoRibbon()

  assert.equal(
    renderSnakePath(anchors, track, track.perimeter),
    renderSnakePath(anchors, track, 0),
  )
})

test('the ribbon bends around the two sharp corners without tearing', () => {
  const factors = stretchFactors(logoRibbon())
  const min = Math.min(...factors)
  const max = Math.max(...factors)

  // Chạy thẳng trên đường bao thì mũi nhọn phải-trên (bán kính 4) kéo hệ
  // số lên 3.55 - dải sóng toạc hình. Đường chạy đi giữa dải phải giữ
  // được trong khoảng uốn cong bình thường.
  assert.ok(min > 0.5, `bị nén tới ${min}`)
  assert.ok(max < 1.6, `bị giãn tới ${max}`)
})

test('running straight on the outline is the case buildRibbon exists to avoid', () => {
  const track = buildTrack(parsePathPoints(LOGO_TRACK_D))
  const anchors = projectOntoTrack(parsePathPoints(LOGO_WAVE_D), track)
  const worst = Math.max(...stretchFactors({ track, anchors }))

  assert.ok(worst > 3, `chỉ giãn ${worst} - nếu hết méo thật thì bỏ buildRibbon đi`)
})

test('closedLength counts the edge back to the first point', () => {
  assert.equal(
    closedLength([
      { x: 0, y: 0 },
      { x: 3, y: 0 },
      { x: 3, y: 4 },
    ]),
    12,
  )
})
