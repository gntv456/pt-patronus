import { describe, expect, it } from 'vitest'

import { applyGroupRead, groupReadNoticeKey } from './noticeGroupRead'

// #294：一键已读的组头不能「无条件标成已读」——逐条结果决定谁算处理完。

interface N {
  id: number
  read?: boolean
}

describe('applyGroupRead', () => {
  it('全部成功：整组标已读、失败 0、未读归零', () => {
    const items: N[] = [{ id: 1 }, { id: 2 }, { id: 3 }]
    const out = applyGroupRead(items, [true, true, true], 3)
    expect(out).toEqual({ marked: 3, failed: 0, unreadLeft: 0 })
    expect(items.every((n) => n.read === true)).toBe(true)
  })

  it('部分失败：只标成功的那几条，未读数按剩余算', () => {
    const items: N[] = [{ id: 1 }, { id: 2 }, { id: 3 }]
    const out = applyGroupRead(items, [true, false, false], 3)
    expect(out).toEqual({ marked: 1, failed: 2, unreadLeft: 2 })
    expect(items.map((n) => n.read)).toEqual([true, undefined, undefined])
  })

  it('正控（旧行为的红点）：全失败时一条都不许被标成已读', () => {
    const items: N[] = [{ id: 1 }, { id: 2 }]
    const out = applyGroupRead(items, [false, false], 2)
    expect(out.marked).toBe(0)
    expect(out.unreadLeft).toBe(2)
    expect(items.every((n) => !n.read)).toBe(true)
  })

  it('剩余未读不会被算成负数', () => {
    const items: N[] = [{ id: 1 }]
    expect(applyGroupRead(items, [true], 0).unreadLeft).toBe(0)
  })
})

describe('groupReadNoticeKey', () => {
  it('三种结果各一句：全成功不出声、部分失败与全失败措辞不同', () => {
    expect(groupReadNoticeKey({ marked: 3, failed: 0, unreadLeft: 0 })).toBe('all')
    expect(groupReadNoticeKey({ marked: 1, failed: 2, unreadLeft: 2 })).toBe('partial')
    expect(groupReadNoticeKey({ marked: 0, failed: 4, unreadLeft: 4 })).toBe('none')
  })
})
