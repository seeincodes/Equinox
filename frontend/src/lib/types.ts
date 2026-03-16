export type Venue = 'KALSHI' | 'POLYMARKET'
export type MarketStatus = 'OPEN' | 'CLOSED' | 'RESOLVED' | 'SUSPENDED'
export type MatchMethod = 'RULE_BASED' | 'EMBEDDING' | 'HYBRID'
export type MatchFlag =
  | 'RESOLUTION_TIME_MISMATCH'
  | 'LOW_CONFIDENCE'
  | 'CATEGORICAL_MISMATCH'
  | 'SETTLEMENT_DIVERGENCE'
  | 'STALE_PRICING_DATA'
  | 'SINGLE_VENUE_ONLY'
  | 'EMBEDDING_UNAVAILABLE'
  | 'RESOLUTION_TIME_MISSING'
export type Role = 'viewer' | 'analyst' | 'admin'

export interface Market {
  id: string
  venue: Venue
  title: string
  normalized_title?: string
  yes_price: number
  no_price: number
  spread: number
  liquidity: number
  status: MarketStatus
  contract_type: string
  settlement_mechanism: string
  data_staleness_flag?: boolean
}

export interface EquivalenceGroup {
  group_id: string
  member_ids: string[]
  members: Market[]
  confidence_score: number
  match_method: MatchMethod
  embedding_similarity: number | null
  string_similarity: number | null
  match_rationale: string
  flags: MatchFlag[]
}

export interface VenueScore {
  price_quality: number
  liquidity: number
  spread_quality: number
  market_status: number
  total: number
}

export interface RejectedVenue {
  venue: Venue
  market_id: string
  score: VenueScore
  rejection_reason: string
}

export interface RoutingDecision {
  decision_id: string
  group_id: string
  order_request: { market_id: string; side: string; size: number }
  selected_venue: Venue
  selected_market_id: string
  rejected_alternatives: RejectedVenue[]
  scoring_breakdown: Record<string, VenueScore>
  routing_rationale: string
  cache_mode: boolean
  created_at: string
  user_id?: string
}

export interface User {
  id: string
  email: string
  role: Role
  created_at: string
}

export interface PriceSnapshot {
  timestamp: string
  venue: Venue
  yes_price: number
  no_price: number
  spread: number
  liquidity: number
}

export interface ConfigMap {
  [key: string]: string
}
