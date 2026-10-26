import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { APICallResultDelivery } from '../../services/callApi';
import { CallDeliverySummary, ResultDeliveries } from './ResultDeliveries';

const file: APICallResultDelivery = {
  id: 41, result_ordinal: 0, delivery_mode: 'managed_copy', state: 'delivery_failed',
  content_type: 'image/png', content_length: 0, available: false, updated_at: '2026-10-20T07:00:00Z',
  reason_code: 'managed_copy_size_exceeded', reason_message: '生成文件超过转存大小限制',
};

describe('call result files', () => {
  it('shows a failed transfer separately from the call status and provides no file link', () => {
    const html = renderToStaticMarkup(<ResultDeliveries callId="call" deliveries={[file]} />);
    expect(html).toContain('转存失败');
    expect(html).toContain('生成文件超过转存大小限制');
    expect(html).not.toContain('href=');
    expect(html).not.toContain('刷新文件地址');
    const summary = renderToStaticMarkup(<CallDeliverySummary deliveries={[file]} onOpen={() => {}} />);
    expect(summary).toContain('个转存失败');
    expect(summary).toContain('查看文件与地址');
  });

  it('does not treat a ready record with an expired or missing asset as a successful transfer', () => {
    const unavailable = { ...file, state: 'ready', reason_message: '转存文件不可用或已清理' };
    const html = renderToStaticMarkup(<ResultDeliveries callId="call" deliveries={[unavailable]} />);
    expect(html).toContain('文件不可用');
    expect(html).not.toContain('转存成功');
    expect(html).not.toContain('刷新文件地址');
  });

  it('identifies referenced results and handles calls without file results', () => {
    const reference = { ...file, state: 'ready', delivery_mode: 'reference', reason_code: '', reason_message: '' };
    const html = renderToStaticMarkup(<ResultDeliveries callId="call" deliveries={[reference]} />);
    expect(html).toContain('没有转存文件');
    expect(html).not.toContain('转存失败');
    expect(renderToStaticMarkup(<ResultDeliveries callId="call" deliveries={[]} />)).toContain('暂无结果文件记录');
  });

  it('offers recovery only when the original result is available and no retry is running', () => {
    const html = renderToStaticMarkup(<ResultDeliveries callId="call" deliveries={[{ ...file, can_retry: true }]} />);
    expect(html).toContain('重新转存');
    expect(html).toContain('不重新生图或扣费');
    const pending = renderToStaticMarkup(<ResultDeliveries callId="call" deliveries={[{ ...file, retry_status: 'pending', retry_attempts: 2, can_retry: false }]} />);
    expect(pending).toContain('等待重新转存');
    expect(pending).not.toContain('>重新转存</button>');
  });
});
