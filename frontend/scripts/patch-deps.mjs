// 对第三方依赖打持久化补丁（npm install 后自动应用；幂等）。
//
// 背景：element-plus 2.14.x 的 useKeyRender.initWatchDom 在 onMounted 无防御地调
//   table.vnode.el.querySelector(".hidden-columns")。当 ElTable 在 el-dialog
//   (Teleport+Transition) / v-if 切换等场景下 mounted 时，vue 3.5 给到的 vnode.el
//   可能是注释占位节点（非 HTMLElement）→ "TypeError: ...querySelector is not a function"，
//   页面渲染中断。上游 2.14.2（当前最新）未修。这里给 el 加防御：非 Element 时直接跳过
//   row-key 顺序监听（功能降级，不崩；正常场景 el 是 HTMLElement，行为不变）。
import { readFileSync, writeFileSync, existsSync } from 'node:fs'

const patches = [
  {
    file: 'node_modules/element-plus/es/components/table/src/table/key-render-helper.mjs',
    find: 'const columnsWrapper = table.vnode.el.querySelector(".hidden-columns");',
    replace:
      'const el = table.vnode.el;\n' +
      '\t\tif (!el || typeof el.querySelector !== "function") return;\n' +
      '\t\tconst columnsWrapper = el.querySelector(".hidden-columns");',
  },
]

for (const p of patches) {
  if (!existsSync(p.file)) {
    console.warn(`[patch-deps] not found, skip: ${p.file}`)
    continue
  }
  const src = readFileSync(p.file, 'utf8')
  if (src.includes(p.replace)) {
    continue // 已应用
  }
  if (!src.includes(p.find)) {
    console.warn(`[patch-deps] find pattern not matched (upstream changed?), skip: ${p.file}`)
    continue
  }
  writeFileSync(p.file, src.replace(p.find, p.replace))
  console.log(`[patch-deps] patched: ${p.file}`)
}
