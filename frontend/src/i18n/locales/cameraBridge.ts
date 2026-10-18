// 摄像头接入指引文案（独立模块，不塞进两张主翻译表）。
//
// 为什么要独立：zh-CN.ts / en-US.ts 都在 scripts/check_file_length.sh 的行数棘轮上
// （只许缩不许涨）。设备监控这类引导性长文案属于「低频变更、随需求增长」的内容，
// 放进主表会让那两张表持续顶线、迟早逼出无意义的代码压缩。与其压缩既有文案，
// 不如把易增长的接入指引按域拆成独立模块，由 i18n/index.ts 合并进 messages。
//
// 键名沿用主表约定「页.区块.元素」：cambridge.* = camera bridge（跨厂商接入）。
export default {
  zh: {
    cambridge: {
      rtspTitle: 'RTSP 直连需要摄像机开放码流',
      rtspGeneric:
        '支持 RTSP 的摄像机（海康、大华、部分群晖、刷了第三方固件的小米/yi 设备）直接填地址即可，填完点「探活」验证。',
      xiaomiLead:
        '小米 / 米家摄像机官方不开放 RTSP 与 ONVIF（码流走 P2P 加密），需先用官方 Miloco 拉流、再用 Micam + go2rtc 转成 RTSP：',
      xiaomiStep1: '部署 Miloco，浏览器打开 https://<miloco 地址>:8000（自签证书需手动信任）',
      xiaomiStep2: '登录后绑定小米账号，确认 Miloco 页面能看到该摄像机画面',
      xiaomiStep3:
        '按 micam 的 docker-compose 起 Micam + go2rtc，取 rtsp://<go2rtc 地址>:8554/<stream> 填入上方「RTSP 地址」',
      codecNote: '小米经 Micam 默认输出 H.265，本系统 H.264/H.265 都支持，无需额外转码。',
      yiHackNote:
        '老款小米 Yi 系列（Hi3518 芯片）也可直接刷 yi-hack v4/v5 固件，自带 RTSP + ONVIF，之后改用「ONVIF」只填 IP 与账号密码即可。',
      onvifTitle: 'ONVIF 会自动发现该摄像机的 RTSP 码流地址',
      onvifBody:
        '填摄像机 IP（可含端口/path）后保存，系统会自动拉取厂商、型号、固件并回填 RTSP 地址，无需手填码流地址。',
      onvifYiHack: '若设备不支持 ONVIF 则探活会失败——小米等消费级设备需先经 Miloco/go2rtc 转 RTSP，或刷 yi-hack 固件。',
      assetsPending: '首次使用，正在准备播放器…',
      assetsPendingHint: '解码器首次使用时自动获取（约 1.4 MB），完成后自动开始播放，无需操作。',
    },
  },
  en: {
    cambridge: {
      rtspTitle: 'RTSP direct connection requires the camera to expose its stream',
      rtspGeneric:
        'Cameras that speak RTSP (Hikvision, Dahua, some Synology, Xiaomi/yi with third-party firmware) work by filling in the address, then hit “Probe”.',
      xiaomiLead:
        'Xiaomi / Mi Home cameras expose neither RTSP nor ONVIF (streams are P2P-encrypted). Pull the stream with the official Miloco, then republish it as RTSP with Micam + go2rtc:',
      xiaomiStep1: 'Deploy Miloco and open https://<miloco address>:8000 (trust the self-signed certificate)',
      xiaomiStep2: 'Sign in, bind your Xiaomi account, and confirm Miloco shows video from the camera',
      xiaomiStep3:
        'Start Micam + go2rtc via the micam docker-compose, then paste rtsp://<go2rtc address>:8554/<stream> into “RTSP 地址” above',
      codecNote:
        'Xiaomi via Micam outputs H.265 by default; this system handles both H.264 and H.265, so no transcoding is needed.',
      yiHackNote:
        'Older Xiaomi Yi series cameras (Hi3518 chipsets) can instead be flashed with yi-hack v4/v5 firmware, which ships RTSP + ONVIF — then switch to “ONVIF” and just fill in IP and credentials.',
      onvifTitle: 'ONVIF auto-discovers the camera’s RTSP stream URL',
      onvifBody:
        'Enter the camera IP (port/path optional) and save. The system fetches vendor, model and firmware, then back-fills the RTSP URL for you.',
      onvifYiHack:
        'If the device does not support ONVIF the probe will fail — consumer cameras like Xiaomi must first be bridged via Miloco/go2rtc, or flashed with yi-hack firmware.',
      assetsPending: 'Preparing the player for the first time…',
      assetsPendingHint:
        'The decoder is fetched automatically on first use (about 1.4 MB), then playback starts on its own. Nothing to do.',
    },
  },
}
