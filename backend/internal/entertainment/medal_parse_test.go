// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import "testing"

// 真实页面结构（源码 medal_synthesis.php 渲染）：
// <div class="synthesis-card" data-synthesis-id="ID"> 内含 <h3>名称、<img>、
// <div class="fragment-item"><span class="current">N</span>/<span class="required">N</span>、
// <span class="pity-count">N/N</span>、drawFragment(ID,1/10/100,price)、<button class="btn-synthesize">。
const medalPageFixture = `<html><body>
<div class="synthesis-card" data-synthesis-id="5">
  <h3>美人鱼</h3>
  <img src="/medals/5.png" alt="美人鱼">
  <div class="fragment-item" data-fragment-id="1" title="鱼尾">
    <img src="/frag/1.png" alt="鱼尾">
    <div class="fragment-count"><span class="current">3</span>/<span class="required">5</span></div>
  </div>
  <div class="fragment-item" data-fragment-id="2" title="鳞片">
    <div class="fragment-count"><span class="current">8</span>/<span class="required">10</span></div>
  </div>
  <span class="pity-count" style="font-weight:bold;">10/300</span>
  <button class="btn btn-draw" onclick="drawFragment(5, 1, 500)">单抽</button>
  <button class="btn btn-draw-ten" onclick="drawFragment(5, 10, 4500)">十连</button>
  <button class="btn btn-draw-hundred" onclick="drawFragment(5, 100, 40000)">百连</button>
  <button class="btn-synthesize">合成</button>
</div>
<div class="synthesis-card" data-synthesis-id="9">
  <h3>彩虹</h3>
  <button class="btn-synthesize" disabled>合成</button>
</div>
</body></html>`

func TestParseSyntheses(t *testing.T) {
	targets := parseSyntheses(medalPageFixture, "https://hxpt.org")
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	m := targets[0]
	if m.ID != 5 || m.Name != "美人鱼" || m.Image != "https://hxpt.org/medals/5.png" {
		t.Errorf("target0 = %+v", m)
	}
	if m.Got != 11 || m.Need != 15 { // min(3,5)+min(8,10)=3+8=11 ; 5+10=15
		t.Errorf("got/need = %d/%d", m.Got, m.Need)
	}
	if m.DrawPrice != 500 || m.TenPrice != 4500 || m.HundredPrice != 40000 {
		t.Errorf("prices = %d/%d/%d", m.DrawPrice, m.TenPrice, m.HundredPrice)
	}
	if !m.CanSynthesize || m.Pity != "10/300" {
		t.Errorf("canSynth=%v pity=%q", m.CanSynthesize, m.Pity)
	}
	if len(m.Fragments) != 2 || m.Fragments[0].Current != 3 || m.Fragments[0].Required != 5 {
		t.Errorf("fragments = %+v", m.Fragments)
	}
	// 第二个目标：合成按钮 disabled → CanSynthesize=false
	if targets[1].CanSynthesize {
		t.Error("target1 should not be synthesizable (disabled)")
	}
}
