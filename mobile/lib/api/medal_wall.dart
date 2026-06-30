import '../core/api_client.dart';

class Medal {
  final int siteId;
  final String siteName;
  final String medalId;
  final String name;
  final String description;
  final String imageSmall;
  final String originalImageUrl;
  final int? saleBeginTime;
  final int? saleEndTime;
  final double price;
  final double originalPrice;
  final String validity;
  final String bonusRate;
  final String purchaseStatus;
  final int stock;
  final String currency;
  final String group;
  final bool wearStatus;
  final bool isOwned;
  final String error;

  Medal({
    required this.siteId,
    required this.siteName,
    required this.medalId,
    required this.name,
    required this.description,
    required this.imageSmall,
    required this.originalImageUrl,
    required this.saleBeginTime,
    required this.saleEndTime,
    required this.price,
    required this.originalPrice,
    required this.validity,
    required this.bonusRate,
    required this.purchaseStatus,
    required this.stock,
    required this.currency,
    required this.group,
    required this.wearStatus,
    required this.isOwned,
    required this.error,
  });

  factory Medal.fromJson(Map<String, dynamic> j) => Medal(
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        siteName: j['site_name'] as String? ?? '',
        medalId: j['medal_id'] as String? ?? '',
        name: j['name'] as String? ?? '',
        description: j['description'] as String? ?? '',
        imageSmall: j['image_small'] as String? ?? '',
        originalImageUrl: j['original_image_url'] as String? ?? '',
        saleBeginTime: j['sale_begin_time'] as int?,
        saleEndTime: j['sale_end_time'] as int?,
        price: (j['price'] as num?)?.toDouble() ?? 0,
        originalPrice: (j['original_price'] as num?)?.toDouble() ?? 0,
        validity: j['validity'] as String? ?? '',
        bonusRate: j['bonus_rate'] as String? ?? '',
        purchaseStatus: j['purchase_status'] as String? ?? 'UNKNOWN',
        stock: (j['stock'] as num?)?.toInt() ?? -1,
        currency: j['currency'] as String? ?? '',
        group: j['group'] as String? ?? '',
        wearStatus: j['wear_status'] as bool? ?? false,
        isOwned: j['is_owned'] as bool? ?? false,
        error: j['error'] as String? ?? '',
      );

  bool get isAvailable => purchaseStatus == 'AVAILABLE';
  bool get isOutOfStock => purchaseStatus == 'OUT_OF_STOCK';
}

class ActionResult {
  final bool success;
  final String message;
  ActionResult({required this.success, required this.message});
  factory ActionResult.fromJson(Map<String, dynamic> j) => ActionResult(
        success: j['success'] as bool? ?? false,
        message: j['message'] as String? ?? '',
      );
}

class MedalWallApi {
  final _api = ApiClient();

  Future<List<Medal>> all({bool refresh = false}) async {
    final r = await _api.get('/medalwall${refresh ? '?refresh=1' : ''}');
    final list = (r['medals'] as List?) ?? const [];
    return list.map((e) => Medal.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<ActionResult> buy(int siteId, String medalId) => _action('buy', siteId, medalId);
  Future<ActionResult> wear(int siteId, String medalId) => _action('wear', siteId, medalId);
  Future<ActionResult> takeOff(int siteId, String medalId) => _action('takeoff', siteId, medalId);

  Future<ActionResult> _action(String action, int siteId, String medalId) async {
    final r = await _api.post('/medalwall/$action', {'site_id': siteId, 'medal_id': medalId});
    return ActionResult.fromJson(r);
  }
}
