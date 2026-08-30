// 从 savept_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/savept/savept_page.dart';

class _NoticeSection {
  final String key;
  final String title;
  final List<String> items;
  _NoticeSection(this.key, this.title, this.items);
}

class _NoticeToken {
  final bool isSite;
  String text;
  _NoticeToken.text(this.text) : isSite = false;
  _NoticeToken.site(String name) : isSite = true, text = name;
}
