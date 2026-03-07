import { create } from "zustand"
import type {
  GraphNode,
  ActiveFilters,
  TimelineRange,
  NodeTypeFilters,
  EdgeStatusFilters,
  ReliabilityFilters,
} from "@/lib/types"

// ─── Default filter state ─────────────────────────────────────────────────────

const defaultNodeTypeFilters: NodeTypeFilters = {
  politician: true,
  scandal: true,
  organization: true,
  legal_proceeding: true,
}

const defaultEdgeStatusFilters: EdgeStatusFilters = {
  convicted: true,
  indicted: true,
  cited: true,
  membership: true,
}

const defaultReliabilityFilters: ReliabilityFilters = {
  high: true,
  medium: true,
  low: true,
}

const defaultFilters: ActiveFilters = {
  nodeTypes: defaultNodeTypeFilters,
  edgeStatus: defaultEdgeStatusFilters,
  reliability: defaultReliabilityFilters,
}

const defaultTimelineRange: TimelineRange = {
  from: 2000,
  to: 2025,
}

// ─── Store interface ──────────────────────────────────────────────────────────

interface AppState {
  // UI state
  isFilterPanelOpen: boolean
  isDetailPanelOpen: boolean

  // Selected node
  selectedNode: GraphNode | null
  focusedNodeId: string | null

  // Search
  searchQuery: string

  // Filters
  filters: ActiveFilters

  // Timeline
  timelineRange: TimelineRange

  // Actions
  setFilterPanelOpen: (open: boolean) => void
  toggleFilterPanel: () => void

  setDetailPanelOpen: (open: boolean) => void
  closeDetailPanel: () => void

  setSelectedNode: (node: GraphNode | null) => void
  setFocusedNodeId: (id: string | null) => void

  setSearchQuery: (query: string) => void

  setNodeTypeFilter: (type: keyof NodeTypeFilters, value: boolean) => void
  setEdgeStatusFilter: (status: keyof EdgeStatusFilters, value: boolean) => void
  setReliabilityFilter: (level: keyof ReliabilityFilters, value: boolean) => void
  resetFilters: () => void

  setTimelineRange: (range: TimelineRange) => void
}

// ─── Store ────────────────────────────────────────────────────────────────────

export const useAppStore = create<AppState>((set) => ({
  isFilterPanelOpen: false,
  isDetailPanelOpen: false,
  selectedNode: null,
  focusedNodeId: null,
  searchQuery: "",
  filters: defaultFilters,
  timelineRange: defaultTimelineRange,

  setFilterPanelOpen: (open) => set({ isFilterPanelOpen: open }),
  toggleFilterPanel: () =>
    set((state) => ({ isFilterPanelOpen: !state.isFilterPanelOpen })),

  setDetailPanelOpen: (open) => set({ isDetailPanelOpen: open }),
  closeDetailPanel: () =>
    set({ isDetailPanelOpen: false, selectedNode: null, focusedNodeId: null }),

  setSelectedNode: (node) =>
    set({
      selectedNode: node,
      isDetailPanelOpen: node !== null,
      focusedNodeId: node?.id ?? null,
    }),

  setFocusedNodeId: (id) => set({ focusedNodeId: id }),

  setSearchQuery: (query) => set({ searchQuery: query }),

  setNodeTypeFilter: (type, value) =>
    set((state) => ({
      filters: {
        ...state.filters,
        nodeTypes: { ...state.filters.nodeTypes, [type]: value },
      },
    })),

  setEdgeStatusFilter: (status, value) =>
    set((state) => ({
      filters: {
        ...state.filters,
        edgeStatus: { ...state.filters.edgeStatus, [status]: value },
      },
    })),

  setReliabilityFilter: (level, value) =>
    set((state) => ({
      filters: {
        ...state.filters,
        reliability: { ...state.filters.reliability, [level]: value },
      },
    })),

  resetFilters: () => set({ filters: defaultFilters }),

  setTimelineRange: (range) => set({ timelineRange: range }),
}))
