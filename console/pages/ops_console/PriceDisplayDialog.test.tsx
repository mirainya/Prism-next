import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { PricePreview } from './PriceDisplayDialog';

describe('price display preview', () => {
  it('shows the final formula, tier thresholds and cap without a second multiplier', () => {
    const html = renderToStaticMarkup(<PricePreview pricing={{ currency: 'CREDIT', variants: [{ operation: 'videos.generate', service_tiers: ['standard'], components: [{ component: 'video', unit: 'second', mode: 'expression', formula: "seconds * tier('rate_1', seconds)", tiers: { rate_1: [{ up_to: '10', value: '0.9' }, { up_to: '', value: '0.72' }] }, maximum: '27' }] }] }} />);
    expect(html).toContain('公开价格预览');
    expect(html).toContain('seconds * tier');
    expect(html).toContain('≤10 → 0.9；其余 → 0.72');
    expect(html).toContain('上限 27');
    expect(html).not.toContain('1.8');
    expect(html).not.toContain('undefined');
  });
});
