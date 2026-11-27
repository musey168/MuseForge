import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle } from '../lib/load-app.mjs';

test('usage chart options expose meaningful series, labels, tooltips, and statistics', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  const result = app.evaluate(`(() => {
    const theme=OBS_THEME_PRESETS.light;
    const opts={theme,reduced:true,fmtTokens:v=>v+' tok',fmtCost:v=>'¥'+Number(v).toFixed(2),label:p=>p.date};
    const points=[
      {date:'2026-09',executions:2,tokens:100,inputTokens:60,outputTokens:20,cacheReadTokens:20,cacheWriteTokens:5,costUsd:1,costCny:7},
      {date:'2026-10',executions:4,tokens:240,inputTokens:120,outputTokens:60,cacheReadTokens:60,cacheWriteTokens:10,costUsd:2.5,costCny:17.5}
    ];
    const trend=OBS_USAGE_TREND_MODES.map(mode=>obsUsageTrendOption(points,mode,opts));
    const breakdown=obsBreakdownOption([{name:'GPT',tokens:75,costCny:21},{name:'GLM',tokens:25,costCny:7}],'cost',opts);
    return {
      trendSeries:trend.map(x=>x.series.length),
      trendNames:trend[0].series.map(s=>s.name),
      trendTip:trend[0].tooltip.formatter([{dataIndex:1}]),
      costSeries:trend[1].series[0].data,
      costTip:trend[1].tooltip.formatter([{dataIndex:1}]),
      execSeries:trend[2].series[0].data,
      costAxis:obsUsageTrendOption(points,'cost',opts).yAxis.axisLabel.formatter(17.5),
      breakdownValues:breakdown.series[0].data.map(p=>p.value),
      breakdownTip:breakdown.tooltip.formatter({name:'GPT',value:21}),
      escaped:obsEsc('<x>'),
      theme:obsEChartsTheme('light').mode
    };
  })()`);
  assert.equal(JSON.stringify(result.trendNames), JSON.stringify(['输入', '输出', '缓存读', '缓存写']));
  assert.equal(result.trendSeries[1], 1, 'cost mode draws one series');
  assert.equal(result.trendSeries[2], 1, 'execution mode draws one series');
  assert.match(result.trendTip, /240 tok|缓存读/);
  assert.equal(JSON.stringify(result.costSeries), JSON.stringify([7, 17.5]));
  assert.match(result.costTip, /¥17\.50/);
  assert.equal(JSON.stringify(result.execSeries), JSON.stringify([2, 4]));
  assert.equal(result.costAxis, '¥17.50');
  assert.equal(JSON.stringify(result.breakdownValues), JSON.stringify([21, 7]));
  assert.match(result.breakdownTip, /¥21\.00/);
  assert.match(result.breakdownTip, /75\.0%/);
  assert.equal(result.escaped, '&lt;x&gt;');
  assert.equal(result.theme, 'light');
});

test('chart lifecycle mounts a concrete chart, routes events, replays, rebuilds, and disposes it', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  app.evaluate(`
    window.__chartLog=[];
    window.IntersectionObserver=undefined;
    const chart={
      disposed:false,
      handlers:{},zr:{handlers:{},on(n,f){this.handlers[n]=f}},
      setOption(o){window.__chartLog.push(['option',o.series[0].data[0]])},
      on(n,f){this.handlers[n]=f},getZr(){return this.zr},
      containPixel(){return true},convertFromPixel(){return [2,0]},
      resize(){window.__chartLog.push(['resize'])},clear(){window.__chartLog.push(['clear'])},
      dispose(){this.disposed=true;window.__chartLog.push(['dispose'])},isDisposed(){return this.disposed}
    };
    window.echarts={init:(el,theme,opt)=>{window.__chartLog.push(['init',opt.renderer]);return chart}};
    const host=document.createElement('div');host.id='chart-host';document.body.append(host);
    host.innerHTML=obsChartSlot('unit',{option:()=>({series:[{data:[7]}]}),on:{click:p=>window.__chartLog.push(['click',p.name])},axisClick:i=>window.__chartLog.push(['axis',i]),mounted:()=>window.__chartLog.push(['mounted'])},180);
    Object.defineProperty(host.firstElementChild,'clientWidth',{value:400,configurable:true});
    obsMountCharts(host);
  `);
  await settle(5);
  app.evaluate(`
    const e=obsChartEntry('unit');e.chart.handlers.click({name:'point'});
    e.chart.zr.handlers.click({offsetX:4,offsetY:5});
    obsReplayChart('unit');obsRebuildCharts();
  `);
  await settle(5);
  assert.equal(app.evaluate(`obsChartCount()`), 1);
  const log = app.evaluate(`window.__chartLog`);
  assert.ok(log.some(x => x[0] === 'init' && x[1] === 'svg'));
  assert.ok(log.some(x => x[0] === 'click' && x[1] === 'point'));
  assert.ok(log.some(x => x[0] === 'axis' && x[1] === 2));
  assert.ok(log.some(x => x[0] === 'clear'));
  app.evaluate(`obsDisposeCharts(document.querySelector('#chart-host'))`);
  assert.equal(app.evaluate(`obsChartCount()`), 0);
});
