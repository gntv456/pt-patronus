// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 workflow_editor_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/workflow/workflow_editor_page.dart';

/// 贝塞尔连线：源节点底中点 → 目标节点顶中点。
class _EdgePainter extends CustomPainter {
  final List<GraphEdge> edges;
  final Map<String, GraphNode> nodes;
  final Size nodeSize;
  final Color color;
  _EdgePainter({
    required this.edges,
    required this.nodes,
    required this.nodeSize,
    required this.color,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color.withValues(alpha: 0.7)
      ..strokeWidth = 2.4
      ..style = PaintingStyle.stroke;
    for (final e in edges) {
      final s = nodes[e.source];
      final t = nodes[e.target];
      if (s == null || t == null) continue;
      final start = Offset(
        s.position.x + nodeSize.width / 2,
        s.position.y + nodeSize.height,
      );
      final end = Offset(t.position.x + nodeSize.width / 2, t.position.y);
      final mid = (start.dy + end.dy) / 2;
      final path = Path()
        ..moveTo(start.dx, start.dy)
        ..cubicTo(start.dx, mid, end.dx, mid, end.dx, end.dy);
      canvas.drawPath(path, paint);
      // 箭头
      const arrow = 6.0;
      canvas.drawPath(
        Path()
          ..moveTo(end.dx, end.dy)
          ..lineTo(end.dx - arrow / 2, end.dy - arrow)
          ..lineTo(end.dx + arrow / 2, end.dy - arrow)
          ..close(),
        paint..style = PaintingStyle.fill,
      );
      paint.style = PaintingStyle.stroke;
    }
  }

  @override
  bool shouldRepaint(covariant _EdgePainter old) =>
      old.edges != edges || old.nodes != nodes || old.color != color;
}
