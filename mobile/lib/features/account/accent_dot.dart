// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 调色盘色点（从 account_page 拆出：该页触及 400 行门禁）。
import 'package:flutter/material.dart';
import 'package:ptpatronus/theme/accent.dart';

/// 调色盘色点：展示浅色态主色，选中描边（对齐 Web GeneralPanel 的 accent-dot）。
class AccentDot extends StatelessWidget {
  final PTAccentPreset preset;
  final bool selected;
  final VoidCallback onTap;

  const AccentDot({super.key,
    required this.preset,
    required this.selected,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: preset.label,
      child: InkWell(
        onTap: onTap,
        customBorder: const CircleBorder(),
        child: Container(
          width: 40,
          height: 40,
          decoration: BoxDecoration(
            color: preset.light,
            shape: BoxShape.circle,
            border: Border.all(
              color: selected
                  ? Theme.of(context).colorScheme.onSurface
                  : Colors.transparent,
              width: 2,
            ),
          ),
          child: selected
              ? const Icon(Icons.check, size: 18, color: Colors.white)
              : null,
        ),
      ),
    );
  }
}
