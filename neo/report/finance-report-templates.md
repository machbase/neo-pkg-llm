# 금융 데이터 HTML 분석 리포트 템플릿

금융 데이터(주가, 환율, 원자재 등)에 적합한 HTML 분석 리포트 템플릿입니다.

## 변수 설명
| 변수 | 설명 | 채우는 주체 |
|------|------|------------|
| {TABLE} | 테이블명 | SQL 결과 |
| {GENERATED_DATE} | 리포트 생성 일시 | 자동 삽입 |
| {TAG_COUNT} | 태그 수 | SQL 결과 |
| {DATA_COUNT} | 총 데이터 건수 | SQL 결과 |
| {TIME_RANGE} | 데이터 시간 범위 | SQL 결과 |
| {TAG_STATS_ROWS} | 태그별 통계 `<tr>` 행 | SQL → LLM 변환 |
| {CHART_DATA_JSON} | 태그별 통계 JSON | SQL → LLM 변환 |
| {TREND_DATA_JSON} | 월별 시계열 JSON | SQL → LLM 변환 |
| {ANALYSIS} | 심층 분석 | LLM 생성 |
| {RECOMMENDATIONS} | 종합 소견 및 권고 | LLM 생성 |

---

### R-1. 금융 데이터 종합 분석 리포트
용도: 금융 데이터(OHLC, 원자재 등)의 가격 추세, 태그별 통계, 거래량 패턴을 차트와 함께 보여주는 심층 분석 보고서입니다.

```html
<!DOCTYPE html>
<html lang="ko">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{TABLE} 금융 데이터 분석 리포트</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: 'Segoe UI', 'Malgun Gothic', sans-serif; background: #eef1f6; color: #1a202c; line-height: 1.7; }
  .page { max-width: 1000px; margin: 0 auto; padding: 40px 32px; }

  .report-header { background: linear-gradient(135deg, #0f2027 0%, #203a43 50%, #2c5364 100%); color: #fff; padding: 48px 40px; border-radius: 16px; margin-bottom: 32px; position: relative; overflow: hidden; }
  .report-header::after { content: ''; position: absolute; top: -50%; right: -20%; width: 400px; height: 400px; background: radial-gradient(circle, rgba(255,255,255,0.05) 0%, transparent 70%); border-radius: 50%; }
  .report-header h1 { font-size: 32px; font-weight: 700; margin-bottom: 8px; position: relative; z-index: 1; }
  .report-header .subtitle { font-size: 16px; opacity: 0.8; margin-bottom: 20px; position: relative; z-index: 1; }
  .report-header .meta-row { display: flex; gap: 24px; font-size: 13px; opacity: 0.7; position: relative; z-index: 1; flex-wrap: wrap; }

  .section { background: #fff; border-radius: 12px; box-shadow: 0 1px 4px rgba(0,0,0,0.06); padding: 32px; margin-bottom: 28px; }
  .section-title { font-size: 18px; font-weight: 700; color: #1a365d; margin-bottom: 20px; display: flex; align-items: center; gap: 10px; }
  .section-title .icon { width: 32px; height: 32px; border-radius: 8px; display: flex; align-items: center; justify-content: center; font-size: 16px; }
  .icon-blue { background: #ebf4ff; color: #2b6cb0; }
  .icon-green { background: #e6fffa; color: #2f855a; }
  .icon-orange { background: #fefcbf; color: #c05621; }
  .icon-purple { background: #faf5ff; color: #6b46c1; }
  .icon-red { background: #fff5f5; color: #c53030; }

  .kpi-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; margin-bottom: 8px; }
  .kpi-card { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); border-radius: 12px; padding: 20px; color: #fff; text-align: center; }
  .kpi-card:nth-child(2) { background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%); }
  .kpi-card:nth-child(3) { background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%); }
  .kpi-card:nth-child(4) { background: linear-gradient(135deg, #43e97b 0%, #38f9d7 100%); }
  .kpi-card .kpi-label { font-size: 12px; text-transform: uppercase; letter-spacing: 1px; opacity: 0.85; margin-bottom: 6px; }
  .kpi-card .kpi-value { font-size: 24px; font-weight: 800; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

  table { width: 100%; border-collapse: separate; border-spacing: 0; font-size: 14px; border-radius: 8px; overflow: hidden; }
  thead th { background: #2d3748; color: #fff; font-weight: 600; padding: 14px 16px; text-align: left; }
  tbody td { padding: 12px 16px; border-bottom: 1px solid #edf2f7; }
  tbody tr:hover { background: #f7fafc; }
  tbody tr:last-child td { border-bottom: none; }
  .num { text-align: right; font-variant-numeric: tabular-nums; font-family: 'Consolas', 'Menlo', monospace; }

  .chart-title { font-size: 14px; font-weight: 600; color: #4a5568; margin-bottom: 12px; }
  .charts-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
  .chart-full { margin-bottom: 24px; }
  canvas { width: 100%; border-radius: 8px; background: #f8f9fb; border: 1px solid #e8ecf1; }

  .analysis-content { color: #4a5568; font-size: 15px; line-height: 1.9; }
  .analysis-content p { margin-bottom: 14px; }
  .analysis-content strong { color: #1a365d; font-weight: 700; }
  .analysis-content ul, .analysis-content ol { margin: 12px 0 16px 24px; }
  .analysis-content li { margin-bottom: 10px; padding-left: 4px; line-height: 1.7; }
  .analysis-content ol li { list-style-type: decimal; }
  .analysis-content li::marker { color: #2b6cb0; font-weight: 700; }

  .chart-wrap { position: relative; overflow: hidden; }
  .tooltip { position: absolute; pointer-events: none; background: rgba(26,32,44,0.92); color: #fff; padding: 8px 14px; border-radius: 8px; font-size: 12px; line-height: 1.6; white-space: nowrap; display: none; z-index: 10; box-shadow: 0 4px 12px rgba(0,0,0,0.25); }
  .crosshair { position: absolute; top: 0; left: 0; right: 0; bottom: 0; pointer-events: none; display: none; z-index: 5; }
  .crosshair-v { width: 1px; background: rgba(102,126,234,0.5); position: absolute; top: 0; height: 100%; }

  .report-footer { text-align: center; padding: 24px; color: #a0aec0; font-size: 12px; border-top: 1px solid #e2e8f0; margin-top: 12px; }

  @media print { body { background: #fff; } .page { padding: 0; } .section { box-shadow: none; border: 1px solid #e2e8f0; } }
  @media (max-width: 768px) { .kpi-grid { grid-template-columns: repeat(2, 1fr); } .charts-grid { grid-template-columns: 1fr; } .page { padding: 16px; } }
</style>
</head>
<body>
<div class="page">

  <div class="report-header">
    <h1>{TABLE} 금융 데이터 분석 리포트</h1>
    <div class="subtitle">Machbase Neo AI 기반 심층 분석 보고서</div>
    <div class="meta-row">
      <span>&#128197; {GENERATED_DATE}</span>
      <span>&#128202; {TAG_COUNT}개 태그 · {DATA_COUNT}건</span>
      <span>&#9200; {TIME_RANGE}</span>
    </div>
  </div>

  <div class="section" style="background:transparent;box-shadow:none;padding:0;">
    <div class="kpi-grid">
      <div class="kpi-card"><div class="kpi-label">테이블</div><div class="kpi-value">{TABLE}</div></div>
      <div class="kpi-card"><div class="kpi-label">태그 수</div><div class="kpi-value">{TAG_COUNT}</div></div>
      <div class="kpi-card"><div class="kpi-label">데이터 건수</div><div class="kpi-value">{DATA_COUNT}</div></div>
      <div class="kpi-card"><div class="kpi-label">분석 기간</div><div class="kpi-value">{TIME_RANGE}</div></div>
    </div>
  </div>

  <!-- Price Trend Chart -->
  <div class="section">
    <div class="section-title"><div class="icon icon-red">&#128200;</div> 가격 추세 ({ROLLUP_LABEL} 평균)</div>
    <div class="chart-full chart-wrap"><canvas id="trendChart" height="300"></canvas><div class="crosshair" id="trendCross"><div class="crosshair-v"></div></div><div class="tooltip" id="trendTip"></div></div>
  </div>

  <!-- Volume Chart -->
  <div class="section">
    <div class="section-title"><div class="icon icon-green">&#128202;</div> 거래량 추세 ({ROLLUP_LABEL} 평균)</div>
    <div class="chart-full chart-wrap"><canvas id="volumeChart" height="220"></canvas><div class="tooltip" id="volumeTip"></div></div>
  </div>

  <!-- Tag Stats -->
  <div class="section">
    <div class="section-title"><div class="icon icon-blue">&#128200;</div> 태그별 통계 요약</div>
    <table>
      <thead><tr>
        <th>태그(NAME)</th><th class="num">건수(COUNT)</th><th class="num">평균(AVG)</th><th class="num">최솟값(MIN)</th><th class="num">최댓값(MAX)</th>
      </tr></thead>
      <tbody>{TAG_STATS_ROWS}</tbody>
    </table>
  </div>

  <!-- Comparison Charts -->
  <div class="section">
    <div class="section-title"><div class="icon icon-green">&#128202;</div> 태그별 비교 분석</div>
    <div class="charts-grid">
      <div class="chart-wrap"><div class="chart-title">태그별 평균(AVG) 비교 <small style="color:#a0aec0;">(volume 제외)</small></div><canvas id="avgChart" height="280"></canvas><div class="tooltip" id="avgTip"></div></div>
      <div class="chart-wrap"><div class="chart-title">태그별 값 분포 (MIN · AVG · MAX) <small style="color:#a0aec0;">(volume 제외)</small></div><canvas id="rangeChart" height="280"></canvas><div class="tooltip" id="rangeTip"></div></div>
    </div>
  </div>

  <!-- Analysis -->
  <div class="section">
    <div class="section-title"><div class="icon icon-orange">&#128270;</div> 심층 분석</div>
    <div class="analysis-content">{ANALYSIS}</div>
  </div>

  <!-- Recommendations -->
  <div class="section">
    <div class="section-title"><div class="icon icon-purple">&#128161;</div> 종합 소견 및 권고사항</div>
    <div class="analysis-content">{RECOMMENDATIONS}</div>
  </div>

  <div class="report-footer">Machbase Neo 데이터 기반으로 생성 되었습니다.</div>
</div>

<script>
(function(){
  var rawStats = {CHART_DATA_JSON};
  var allStats = rawStats.map(function(d) {
    return { name: d.name||d.tag||'', avg: Number(d.avg)||0, min: Number(d.min)||0, max: Number(d.max)||0, count: Number(d.count)||0 };
  });
  // Exclude volume from comparison charts (different scale)
  var stats = allStats.filter(function(d){ return d.name.toLowerCase()!=='volume'; });

  var rawTrend = {TREND_DATA_JSON};
  var trend = rawTrend.map(function(d) {
    var t = d.time||d.t||'';
    return { time: t, close: Number(d.close||d.v)||0, volume: Number(d.volume)||0 };
  });

  var colors = ['#667eea','#f5576c','#4facfe','#43e97b','#ff9a9e','#a18cd1'];
  var dpr = window.devicePixelRatio || 1;

  function setup(id,h){var c=document.getElementById(id);if(!c)return null;var w=c.parentElement.getBoundingClientRect().width;c.width=w*dpr;c.height=h*dpr;c.style.width=w+'px';c.style.height=h+'px';var ctx=c.getContext('2d');ctx.scale(dpr,dpr);return{ctx:ctx,w:w,h:h,canvas:c};}
  function niceMax(v){if(v<=0)return 1;var p=Math.pow(10,Math.floor(Math.log10(v)));return Math.ceil(v/p)*p;}
  function fmt(v){return v>=10000?(v/1000).toFixed(0)+'K':v>=1000?v.toLocaleString(undefined,{maximumFractionDigits:1}):v.toFixed(2);}

  // Tooltip helper
  function addTip(canvasId,tipId,pts){
    var cv=document.getElementById(canvasId),tip=document.getElementById(tipId);
    if(!cv||!tip||!pts.length)return;
    var cross=document.getElementById(canvasId.replace('Chart','Cross'));
    cv.style.cursor='crosshair';
    cv.addEventListener('mousemove',function(e){
      var r=cv.getBoundingClientRect(),mx=e.clientX-r.left,my=e.clientY-r.top;
      var best=null,bd=Infinity;
      pts.forEach(function(p){var d=Math.abs(p.x-mx);if(d<bd){bd=d;best=p;}});
      if(best&&bd<50){
        tip.innerHTML=best.label;tip.style.display='block';
        var tx=best.x+14;if(tx+160>r.width)tx=best.x-160;
        tip.style.left=tx+'px';tip.style.top=Math.max(4,best.y-24)+'px';
        if(cross){cross.style.display='block';cross.querySelector('.crosshair-v').style.left=best.x+'px';}
      }else{tip.style.display='none';if(cross)cross.style.display='none';}
    });
    cv.addEventListener('mouseleave',function(){tip.style.display='none';if(cross)cross.style.display='none';});
  }

  // === PRICE TREND LINE CHART (with scroll zoom) ===
  // Hide section if no trend data
  if(!trend.length){var tc=document.getElementById('trendChart');if(tc){var sec=tc.closest('.section');if(sec)sec.style.display='none';}}
  var trendState={start:0,end:trend.length,zoomed:false};
  function drawTrend(){
    var slice=trend.slice(trendState.start,trendState.end);
    if(!slice.length)return;
    var c=setup('trendChart',300);if(!c)return;
    var ctx=c.ctx,W=c.w,H=c.h,pad={t:30,r:30,b:50,l:65};
    var cw=W-pad.l-pad.r,ch=H-pad.t-pad.b;
    var vals=slice.map(function(d){return d.close;});
    var mn=Math.min.apply(null,vals),mx=niceMax(Math.max.apply(null,vals)*1.05);
    if(mn>0)mn=mn*0.95;var range=mx-mn||1;
    var n=slice.length,step=cw/(n-1||1);

    ctx.clearRect(0,0,W,H);
    // Grid
    ctx.strokeStyle='#e8ecf1';ctx.lineWidth=1;
    for(var i=0;i<=5;i++){var y=pad.t+ch-(ch*i/5);ctx.beginPath();ctx.moveTo(pad.l,y);ctx.lineTo(W-pad.r,y);ctx.stroke();ctx.fillStyle='#8e99a4';ctx.font='11px Segoe UI';ctx.textAlign='right';ctx.fillText(fmt(mn+range*i/5),pad.l-10,y+4);}
    // Area
    ctx.beginPath();ctx.moveTo(pad.l,pad.t+ch);
    for(var i=0;i<n;i++){ctx.lineTo(pad.l+step*i,pad.t+ch-((vals[i]-mn)/range*ch));}
    ctx.lineTo(pad.l+step*(n-1),pad.t+ch);ctx.closePath();
    var g=ctx.createLinearGradient(0,pad.t,0,pad.t+ch);g.addColorStop(0,'rgba(102,126,234,0.25)');g.addColorStop(1,'rgba(102,126,234,0.02)');ctx.fillStyle=g;ctx.fill();
    // Line
    ctx.beginPath();for(var i=0;i<n;i++){var x=pad.l+step*i,y=pad.t+ch-((vals[i]-mn)/range*ch);if(i===0)ctx.moveTo(x,y);else ctx.lineTo(x,y);}
    ctx.strokeStyle='#667eea';ctx.lineWidth=2.5;ctx.stroke();
    // Key dots
    var miI=vals.indexOf(Math.min.apply(null,vals)),maI=vals.indexOf(Math.max.apply(null,vals));
    [0,n-1,miI,maI].forEach(function(idx){var x=pad.l+step*idx,y=pad.t+ch-((vals[idx]-mn)/range*ch);ctx.beginPath();ctx.arc(x,y,4,0,Math.PI*2);ctx.fillStyle='#667eea';ctx.fill();ctx.fillStyle='#2d3748';ctx.font='bold 11px Segoe UI';ctx.textAlign='center';ctx.fillText(fmt(vals[idx]),x,y-10);});
    // X labels
    ctx.fillStyle='#8e99a4';ctx.font='10px Segoe UI';ctx.textAlign='center';
    var ls=Math.max(1,Math.floor(n/8));
    for(var i=0;i<n;i+=ls)ctx.fillText(slice[i].time,pad.l+step*i,H-pad.b+18);
    if((n-1)%ls!==0)ctx.fillText(slice[n-1].time,pad.l+step*(n-1),H-pad.b+18);
    // Axis
    ctx.strokeStyle='#cbd5e0';ctx.lineWidth=1.5;ctx.beginPath();ctx.moveTo(pad.l,pad.t+ch);ctx.lineTo(W-pad.r,pad.t+ch);ctx.stroke();
    // Zoom hint
    ctx.fillStyle='#a0aec0';ctx.font='10px Segoe UI';ctx.textAlign='left';
    ctx.fillText(trendState.zoomed?'Double-click to reset':'Scroll to zoom',pad.l,pad.t-10);
    ctx.fillStyle='#667eea';ctx.font='bold 12px Segoe UI';ctx.fillText('Close Price',pad.l+120,pad.t-10);

    // Tooltip data
    var pts=slice.map(function(d,i){var x=pad.l+step*i,y=pad.t+ch-((d.close-mn)/range*ch);return{x:x,y:y,label:'<strong>'+d.time+'</strong><br>Close: '+d.close.toLocaleString(undefined,{maximumFractionDigits:2})+(d.volume?'<br>Volume: '+d.volume.toLocaleString():'')};});
    addTip('trendChart','trendTip',pts);
  }
  drawTrend();

  // Scroll zoom on trend chart
  (function(){
    var cv=document.getElementById('trendChart');if(!cv)return;
    cv.addEventListener('wheel',function(e){
      e.preventDefault();
      var rect=cv.getBoundingClientRect();
      var mx=e.clientX-rect.left,pad={l:65,r:30};
      var cw=rect.width-pad.l-pad.r;
      var n=trendState.end-trendState.start;
      // Mouse position as ratio within chart area
      var ratio=Math.max(0,Math.min(1,(mx-pad.l)/cw));
      var zoomFactor=e.deltaY>0?1.3:0.7; // scroll down=zoom out, up=zoom in
      var newN=Math.round(n*zoomFactor);
      if(newN<3)newN=3;
      if(newN>=trend.length){trendState.start=0;trendState.end=trend.length;trendState.zoomed=false;drawTrend();return;}
      var center=trendState.start+Math.round(n*ratio);
      var newStart=Math.round(center-newN*ratio);
      if(newStart<0)newStart=0;
      var newEnd=newStart+newN;
      if(newEnd>trend.length){newEnd=trend.length;newStart=newEnd-newN;}
      trendState.start=Math.max(0,newStart);trendState.end=newEnd;trendState.zoomed=true;
      drawTrend();
    },{passive:false});
    cv.addEventListener('dblclick',function(){trendState.start=0;trendState.end=trend.length;trendState.zoomed=false;drawTrend();});
  })();

  // === VOLUME BAR CHART (hide section if no data) ===
  (function(){
    var vd=trend.filter(function(d){return d.volume>0;});
    if(!vd.length){
      // Hide the volume chart section entirely
      var vc=document.getElementById('volumeChart');
      if(vc){var sec=vc.closest('.section');if(sec)sec.style.display='none';}
      return;
    }
    var c=setup('volumeChart',220);if(!c)return;
    var ctx=c.ctx,W=c.w,H=c.h,pad={t:20,r:30,b:50,l:65};
    var cw=W-pad.l-pad.r,ch=H-pad.t-pad.b,n=vd.length;
    var maxV=niceMax(Math.max.apply(null,vd.map(function(d){return d.volume;}))*1.1);
    var bw=Math.max(2,cw/n-2);
    ctx.strokeStyle='#e8ecf1';ctx.lineWidth=1;
    for(var i=0;i<=4;i++){var y=pad.t+ch-(ch*i/4);ctx.beginPath();ctx.moveTo(pad.l,y);ctx.lineTo(W-pad.r,y);ctx.stroke();ctx.fillStyle='#8e99a4';ctx.font='11px Segoe UI';ctx.textAlign='right';ctx.fillText(fmt(maxV*i/4),pad.l-10,y+4);}
    vd.forEach(function(d,i){var x=pad.l+(cw/n)*i,bh=(d.volume/maxV)*ch,y=pad.t+ch-bh;var g=ctx.createLinearGradient(0,y,0,pad.t+ch);g.addColorStop(0,'#43e97b');g.addColorStop(1,'#38f9d7');ctx.fillStyle=g;ctx.fillRect(x,y,bw,bh);});
    ctx.fillStyle='#8e99a4';ctx.font='10px Segoe UI';ctx.textAlign='center';
    var ls=Math.max(1,Math.floor(n/8));for(var i=0;i<n;i+=ls)ctx.fillText(vd[i].time,pad.l+(cw/n)*i+bw/2,H-pad.b+18);
    ctx.strokeStyle='#cbd5e0';ctx.lineWidth=1.5;ctx.beginPath();ctx.moveTo(pad.l,pad.t+ch);ctx.lineTo(W-pad.r,pad.t+ch);ctx.stroke();
    var pts=vd.map(function(d,i){var x=pad.l+(cw/n)*i+bw/2,bh=(d.volume/maxV)*ch;return{x:x,y:pad.t+ch-bh,label:'<strong>'+d.time+'</strong><br>Volume: '+d.volume.toLocaleString()};});
    addTip('volumeChart','volumeTip',pts);
  })();

  // === AVG BAR CHART (no volume) ===
  (function(){
    if(!stats.length){var ac=document.getElementById('avgChart');if(ac){var p=ac.closest('.chart-wrap');if(p)p.style.display='none';}return;}
    var c=setup('avgChart',280);if(!c)return;
    var ctx=c.ctx,W=c.w,H=c.h,pad={t:20,r:20,b:40,l:60};
    var cw=W-pad.l-pad.r,ch=H-pad.t-pad.b;
    var maxVal=niceMax(Math.max.apply(null,stats.map(function(d){return d.avg;}))*1.15);
    var n=stats.length,gap=cw/n,bw=Math.min(gap*0.6,48);
    ctx.strokeStyle='#e8ecf1';ctx.lineWidth=1;
    for(var i=0;i<=5;i++){var y=pad.t+ch-(ch*i/5);ctx.beginPath();ctx.moveTo(pad.l,y);ctx.lineTo(W-pad.r,y);ctx.stroke();ctx.fillStyle='#8e99a4';ctx.font='11px Segoe UI';ctx.textAlign='right';ctx.fillText(fmt(maxVal*i/5),pad.l-8,y+4);}
    var barPts=[];
    stats.forEach(function(d,i){var x=pad.l+gap*i+(gap-bw)/2,bh=(d.avg/maxVal)*ch,y=pad.t+ch-bh,col=colors[i%colors.length];ctx.fillStyle=col;var r=Math.min(4,bw/4);ctx.beginPath();ctx.moveTo(x,y+r);ctx.arcTo(x,y,x+r,y,r);ctx.arcTo(x+bw,y,x+bw,y+r,r);ctx.lineTo(x+bw,pad.t+ch);ctx.lineTo(x,pad.t+ch);ctx.closePath();ctx.fill();ctx.fillStyle='#2d3748';ctx.font='bold 11px Segoe UI';ctx.textAlign='center';ctx.fillText(fmt(d.avg),x+bw/2,y-6);ctx.fillStyle='#4a5568';ctx.font='12px Segoe UI';ctx.fillText(d.name,pad.l+gap*i+gap/2,H-pad.b+18);
      barPts.push({x:x+bw/2,y:y,label:'<strong>'+d.name+'</strong><br>AVG: '+fmt(d.avg)+'<br>COUNT: '+d.count.toLocaleString()});
    });
    ctx.strokeStyle='#cbd5e0';ctx.lineWidth=1.5;ctx.beginPath();ctx.moveTo(pad.l,pad.t+ch);ctx.lineTo(W-pad.r,pad.t+ch);ctx.stroke();
    addTip('avgChart','avgTip',barPts);
  })();

  // === RANGE CHART (no volume) ===
  (function(){
    if(!stats.length){var rc=document.getElementById('rangeChart');if(rc){var p=rc.closest('.chart-wrap');if(p)p.style.display='none';}return;}
    var c=setup('rangeChart',280);if(!c)return;
    var ctx=c.ctx,W=c.w,H=c.h,pad={t:20,r:20,b:40,l:60};
    var cw=W-pad.l-pad.r,ch=H-pad.t-pad.b;
    var allV=[];stats.forEach(function(d){allV.push(d.min,d.max);});
    var gMin=Math.min.apply(null,allV)*0.95,gMax=niceMax(Math.max.apply(null,allV)*1.05);
    var range=gMax-gMin||1,n=stats.length,gap=cw/n;
    ctx.strokeStyle='#e8ecf1';ctx.lineWidth=1;
    for(var i=0;i<=5;i++){var y=pad.t+ch-(ch*i/5),val=gMin+range*i/5;ctx.beginPath();ctx.moveTo(pad.l,y);ctx.lineTo(W-pad.r,y);ctx.stroke();ctx.fillStyle='#8e99a4';ctx.font='11px Segoe UI';ctx.textAlign='right';ctx.fillText(fmt(val),pad.l-8,y+4);}
    var rPts=[];
    stats.forEach(function(d,i){
      var cx=pad.l+gap*i+gap/2,yMi=pad.t+ch-((d.min-gMin)/range*ch),yMa=pad.t+ch-((d.max-gMin)/range*ch),yAv=pad.t+ch-((d.avg-gMin)/range*ch),col=colors[i%colors.length];
      ctx.strokeStyle=col;ctx.lineWidth=3;ctx.beginPath();ctx.moveTo(cx,yMi);ctx.lineTo(cx,yMa);ctx.stroke();
      ctx.beginPath();ctx.moveTo(cx-8,yMi);ctx.lineTo(cx+8,yMi);ctx.stroke();
      ctx.beginPath();ctx.moveTo(cx-8,yMa);ctx.lineTo(cx+8,yMa);ctx.stroke();
      ctx.fillStyle=col;ctx.beginPath();ctx.moveTo(cx,yAv-5);ctx.lineTo(cx+5,yAv);ctx.lineTo(cx,yAv+5);ctx.lineTo(cx-5,yAv);ctx.closePath();ctx.fill();
      ctx.fillStyle='#2d3748';ctx.font='bold 10px Segoe UI';ctx.textAlign='center';ctx.fillText(fmt(d.avg),cx,yAv-10);
      ctx.fillStyle='#4a5568';ctx.font='12px Segoe UI';ctx.fillText(d.name,cx,H-pad.b+18);
      rPts.push({x:cx,y:yAv,label:'<strong>'+d.name+'</strong><br>MIN: '+fmt(d.min)+'<br>AVG: '+fmt(d.avg)+'<br>MAX: '+fmt(d.max)});
    });
    // Legend
    ctx.font='11px Segoe UI';ctx.fillStyle='#8e99a4';var lx=pad.l+10;
    ctx.strokeStyle='#667eea';ctx.lineWidth=2;ctx.beginPath();ctx.moveTo(lx,H-8);ctx.lineTo(lx+16,H-8);ctx.stroke();ctx.fillText('MIN~MAX',lx+20,H-4);
    ctx.fillStyle='#667eea';ctx.beginPath();ctx.moveTo(lx+100,H-12);ctx.lineTo(lx+104,H-8);ctx.lineTo(lx+100,H-4);ctx.lineTo(lx+96,H-8);ctx.closePath();ctx.fill();
    ctx.fillStyle='#8e99a4';ctx.fillText('AVG',lx+108,H-4);
    ctx.strokeStyle='#cbd5e0';ctx.lineWidth=1.5;ctx.beginPath();ctx.moveTo(pad.l,pad.t+ch);ctx.lineTo(W-pad.r,pad.t+ch);ctx.stroke();
    addTip('rangeChart','rangeTip',rPts);
  })();
})();
</script>
</body>
</html>
```
