import { request } from './request';

export interface PriceDisplaySettings {
  default_multiplier: string;
  model_multipliers: Record<string, string>;
  sku_multipliers: Record<string, string>;
  config_version: number;
}

export interface PriceDisplayPreview {
  currency: string;
  variants: Array<{
    operation: string;
    service_tiers: string[];
    components: Array<{
      component: string;
      unit: string;
      unit_price?: string;
      unit_scale?: number;
      mode: string;
      formula?: string;
      tiers?: Record<string, Array<{ up_to: string; value: string }>>;
      maximum?: string;
    }>;
  }>;
}

const path = '/admin/unified-gateway/price-display';
export const fetchPriceDisplaySettings = (signal?: AbortSignal) =>
  request<PriceDisplaySettings>(path, { signal });
export const savePriceDisplaySettings = (settings: PriceDisplaySettings) =>
  request<PriceDisplaySettings>(path, { method: 'PUT', body: JSON.stringify(settings) });
export const previewPriceDisplay = (settings: PriceDisplaySettings, model: string, signal?: AbortSignal) =>
  request<PriceDisplayPreview>(`${path}/preview`, { method: 'POST', body: JSON.stringify({ settings, model }), signal });
