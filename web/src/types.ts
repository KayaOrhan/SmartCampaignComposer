export type Channel = 'sms' | 'email'
export type Status = 'draft' | 'content_ready' | 'approved' | 'sending' | 'completed' | 'cancelled'
export interface Segment { version: 1; all: Array<{ field: string; op: string; value: string[] | number }> }
export interface Campaign { id: number; revision: number; brief: string; channels: Channel[]; segment: Segment | null; status: Status; created_at: string }
export interface CampaignPage { campaigns: Campaign[]; next_before: number | null }
export interface Bounds { count: number; max_first_name_runes: number; max_city_runes: number }
export interface Audience { as_of: string; eligible_unique: number; by_channel: Partial<Record<Channel, Bounds>> }
export interface Preview extends Audience { revision: number; approval_fingerprint: string | null }
export interface Messages { sms?: { body: string }; email?: { subject: string; body: string } }
export interface SavedMessages { campaign_id: number; revision: number; status: Status; messages: Messages | null; validation: { revision: number; audience: Audience } | null }
export interface Counts { total: number; pending: number; in_flight: number; sent: number; dead: number; skipped: number }
export interface Delivery { campaign_id: number; revision: number; status: Status; counts: Counts; by_channel: Partial<Record<Channel, Counts>>; approved_at: string | null; approved_revision: number | null }
export interface Candidate { attempt: number; base_revision: number; messages: Messages; audience: Audience }
export interface Delta { attempt: number; channel?: Channel; field?: 'body' | 'subject'; text?: string }
