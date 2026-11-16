import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { APICallDeliverySummary } from '../../services/callApi';
import { CallDeliveryBadge } from './CallDeliveryBadge';

const empty: APICallDeliverySummary = { total: 0, ready: 0, processing: 0, checking: 0, failed: 0, unavailable: 0, expired: 0, reference: 0 };
const render = (counts: Partial<APICallDeliverySummary>) => renderToStaticMarkup(<CallDeliveryBadge summary={{ ...empty, ...counts }} onOpen={() => {}} />);

describe('call list transfer results', () => {
  it('shows partial failures and successful file counts without opening the call', () => {
    const html = render({ total: 3, ready: 2, failed: 1 });
    expect(html).toContain('部分转存失败');
    expect(html).toContain('已转存 2/3');
    expect(html).toContain('2 个成功，1 个失败');
    expect(html).toContain('查看转存结果：部分转存失败');
  });

  it('distinguishes checking an existing transfer from a stopped failure', () => {
    expect(render({ total: 1, processing: 1, checking: 1 })).toContain('核对中');
    expect(render({ total: 1, processing: 1 })).toContain('转存中');
    expect(render({ total: 1, failed: 1 })).toContain('转存失败');
  });

  it('does not call missing assets or untransferred links a success', () => {
    expect(render({ total: 1, unavailable: 1 })).toContain('文件不可用');
    expect(render({ total: 1, expired: 1 })).toContain('文件已过期');
    expect(render({ total: 1, reference: 1 })).toContain('未转存');
    expect(render({ total: 1, reference: 1 })).not.toContain('转存成功');
    expect(render({ total: 1, ready: 1 })).toContain('转存成功');
    expect(render({})).toContain('暂无文件');
  });
});
