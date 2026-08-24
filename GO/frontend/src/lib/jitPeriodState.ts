export interface JITPeriodRequest {
  generation: number
  sourceId: string
}

export interface JITPeriodState {
  generation: number
  periodBySource: Record<string, string>
  pending: JITPeriodRequest | null
}

export function createJITPeriodState(): JITPeriodState {
  return { generation: 0, periodBySource: {}, pending: null }
}

export function resetJITPeriodState(state: JITPeriodState): JITPeriodState {
  return { generation: state.generation + 1, periodBySource: {}, pending: null }
}

export function beginJITPeriodUpdate(
  state: JITPeriodState,
  sourceId: string,
): { state: JITPeriodState; request: JITPeriodRequest | null } {
  if (state.pending) return { state, request: null }
  const request = { generation: state.generation, sourceId }
  return { state: { ...state, pending: request }, request }
}

function isCurrentRequest(state: JITPeriodState, request: JITPeriodRequest): boolean {
  return state.generation === request.generation
    && state.pending?.generation === request.generation
    && state.pending.sourceId === request.sourceId
}

export function completeJITPeriodUpdate(
  state: JITPeriodState,
  request: JITPeriodRequest,
  period?: string,
): { state: JITPeriodState; accepted: boolean } {
  if (!isCurrentRequest(state, request)) return { state, accepted: false }
  return {
    state: {
      ...state,
      periodBySource: period === undefined
        ? state.periodBySource
        : { ...state.periodBySource, [request.sourceId]: period },
      pending: null,
    },
    accepted: true,
  }
}

export function isJITPeriodMenuDisabled(isProcessing: boolean, state: JITPeriodState): boolean {
  return isProcessing || state.pending !== null
}
