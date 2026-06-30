// 娱乐模型解析测试：匹配后端 /entertainment 响应形状（含缺字段容错）。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/entertainment.dart';

void main() {
  test('Game.fromJson parses id + name', () {
    final g = Game.fromJson({'id': 'jgg', 'name': '九宫格'});
    expect(g.id, 'jgg');
    expect(g.name, '九宫格');
  });

  test('Game.fromJson tolerates missing fields + numeric id', () {
    final g = Game.fromJson({'id': 42});
    expect(g.id, '42'); // 数字 id 也能解析为字符串
    expect(g.name, '');
  });

  test('PlayResult.fromJson parses win with prize/amount/balance', () {
    final r = PlayResult.fromJson({
      'success': true,
      'prize': '一等奖',
      'amount': 100,
      'balance': 250.5,
      'message': '恭喜中奖',
    });
    expect(r.success, isTrue);
    expect(r.prize, '一等奖');
    expect(r.amount, 100);
    expect(r.balance, 250.5);
    expect(r.message, '恭喜中奖');
  });

  test('PlayResult.fromJson tolerates missing fields (fail by default)', () {
    final r = PlayResult.fromJson({});
    expect(r.success, isFalse);
    expect(r.prize, '');
    expect(r.amount, 0);
    expect(r.balance, 0);
  });
}
