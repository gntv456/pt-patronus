// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Docker Compose 多容器模板库 —— 预置常用编排方案，一键部署。
// NAS 模板库：小白用户选模板 → 微调 → 部署，无需手写 YAML。

export interface ComposeTemplate {
  id: string
  name: string
  category: ComposeTemplateCategory
  icon: string
  color: string
  description: string
  services: string[]
  content: string
}

export type ComposeTemplateCategory = '下载影音' | '媒体中心' | '开发运维' | '智能家居' | '数据库' | '建站'

export const composeTemplateCategories: { key: ComposeTemplateCategory | '全部'; icon: string }[] = [
  { key: '全部', icon: 'home-city' },
  { key: '下载影音', icon: 'download' },
  { key: '媒体中心', icon: 'film' },
  { key: '数据库', icon: 'database' },
  { key: '开发运维', icon: 'terminal' },
  { key: '智能家居', icon: 'home-city' },
  { key: '建站', icon: 'globe' },
]

const D = (p: string) => `/opt/docker/${p}`

export const composeTemplates: ComposeTemplate[] = [
  // ===== 下载影音 =====
  {
    id: 'media-stack',
    name: '影音下载套件（qBittorrent + Jellyfin）',
    category: '下载影音',
    icon: 'film',
    color: '#e50914',
    description: 'BT/PT 下载 + 媒体库自动整理播放，共享下载目录',
    services: ['qbittorrent', 'jellyfin'],
    content: `services:
  qbittorrent:
    image: linuxserver/qbittorrent:latest
    container_name: qbittorrent
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Asia/Shanghai
      - WEBUI_PORT=8080
    volumes:
      - ${D('qbittorrent/config')}:/config
      - ${D('downloads')}:/downloads
    ports:
      - 8080:8080
      - 6881:6881
      - 6881:6881/udp
    restart: unless-stopped

  jellyfin:
    image: jellyfin/jellyfin:latest
    container_name: jellyfin
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Asia/Shanghai
    volumes:
      - ${D('jellyfin/config')}:/config
      - ${D('jellyfin/cache')}:/cache
      - ${D('downloads')}:/media
    ports:
      - 8096:8096
    restart: unless-stopped`,
  },
  {
    id: 'transmission-plex',
    name: 'Transmission + Plex 媒体方案',
    category: '下载影音',
    icon: 'play',
    color: '#e5a00d',
    description: '轻量 BT 下载 + Plex 媒体服务器，适合低配设备',
    services: ['transmission', 'plex'],
    content: `services:
  transmission:
    image: linuxserver/transmission:latest
    container_name: transmission
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Asia/Shanghai
    volumes:
      - ${D('transmission/config')}:/config
      - ${D('downloads')}:/downloads
    ports:
      - 9091:9091
      - 51413:51413
      - 51413:51413/udp
    restart: unless-stopped

  plex:
    image: linuxserver/plex:latest
    container_name: plex
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Asia/Shanghai
      - VERSION=docker
    volumes:
      - ${D('plex/config')}:/config
      - ${D('plex/transcode')}:/transcode
      - ${D('downloads')}:/data
    ports:
      - 32400:32400
    restart: unless-stopped`,
  },
  {
    id: 'aria2-ariang',
    name: 'Aria2 + AriaNg 下载面板',
    category: '下载影音',
    icon: 'zap',
    color: '#16a34a',
    description: 'Aria2 下载引擎 + AriaNg Web 管理界面，支持 HTTP/BT/磁力',
    services: ['aria2', 'ariang'],
    content: `services:
  aria2:
    image: p3terx/aria2-pro:latest
    container_name: aria2
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Asia/Shanghai
      - SECRET=your_aria2_rpc_secret
    volumes:
      - ${D('aria2/config')}:/config
      - ${D('downloads')}:/downloads
    ports:
      - 6800:6800
    restart: unless-stopped

  ariang:
    image: p3terx/ariang:latest
    container_name: ariang
    ports:
      - 6880:6880
    restart: unless-stopped`,
  },

  // ===== 媒体中心 =====
  {
    id: 'navidrome',
    name: 'Navidrome 音乐服务器',
    category: '媒体中心',
    icon: 'music',
    color: '#ef4444',
    description: '自托管音乐流媒体服务器，兼容 Subsonic API',
    services: ['navidrome'],
    content: `services:
  navidrome:
    image: deluan/navidrome:latest
    container_name: navidrome
    environment:
      - ND_SCANNERPARSERHOUR=1
      - ND_LOGLEVEL=info
    volumes:
      - ${D('navidrome/data')}:/data
      - ${D('music')}:/music:ro
    ports:
      - 4533:4533
    restart: unless-stopped`,
  },
  {
    id: 'photoprism',
    name: 'PhotoPrism 相册管理',
    category: '媒体中心',
    icon: 'image',
    color: '#0ea5e9',
    description: 'AI 驱动的个人照片管理，自动分类和人脸识别',
    services: ['photoprism', 'mariadb'],
    content: `services:
  photoprism:
    image: photoprism/photoprism:latest
    container_name: photoprism
    environment:
      - PHOTOPRISM_ADMIN_PASSWORD=insecure
      - PHOTOPRISM_DATABASE_DRIVER=mariadb
      - PHOTOPRISM_DATABASE_SERVER=mariadb:3306
      - PHOTOPRISM_DATABASE_NAME=photoprism
      - PHOTOPRISM_DATABASE_USER=photoprism
      - PHOTOPRISM_DATABASE_PASSWORD=photoprism
    volumes:
      - ${D('photoprism/originals')}:/photoprism/originals
      - ${D('photoprism/storage')}:/photoprism/storage
    ports:
      - 2342:2342
    depends_on:
      - mariadb
    restart: unless-stopped

  mariadb:
    image: mariadb:latest
    container_name: photoprism-db
    environment:
      - MARIADB_ROOT_PASSWORD=photoprism
      - MARIADB_DATABASE=photoprism
      - MARIADB_USER=photoprism
      - MARIADB_PASSWORD=photoprism
    volumes:
      - ${D('photoprism/database')}:/var/lib/mysql
    restart: unless-stopped`,
  },

  // ===== 数据库 =====
  {
    id: 'mysql-redis',
    name: 'MySQL + Redis 开发数据库',
    category: '数据库',
    icon: 'database',
    color: '#00758f',
    description: 'MySQL 8 + Redis 7 常用数据库组合，含 phpMyAdmin 管理面板',
    services: ['mysql', 'redis', 'phpmyadmin'],
    content: `services:
  mysql:
    image: mysql:8
    container_name: mysql
    environment:
      - MYSQL_ROOT_PASSWORD=change_me_password
      - MYSQL_DATABASE=app
      - TZ=Asia/Shanghai
    volumes:
      - ${D('mysql/data')}:/var/lib/mysql
    ports:
      - 3306:3306
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    container_name: redis
    command: redis-server --requirepass change_me_password --appendonly yes
    volumes:
      - ${D('redis/data')}:/data
    ports:
      - 6379:6379
    restart: unless-stopped

  phpmyadmin:
    image: phpmyadmin:latest
    container_name: phpmyadmin
    environment:
      - PMA_HOST=mysql
    ports:
      - 8081:80
    depends_on:
      - mysql
    restart: unless-stopped`,
  },
  {
    id: 'postgres',
    name: 'PostgreSQL 数据库',
    category: '数据库',
    icon: 'database',
    color: '#336791',
    description: 'PostgreSQL 16 + pgAdmin 图形管理界面',
    services: ['postgres', 'pgadmin'],
    content: `services:
  postgres:
    image: postgres:16-alpine
    container_name: postgres
    environment:
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=change_me_password
      - POSTGRES_DB=app
      - TZ=Asia/Shanghai
    volumes:
      - ${D('postgres/data')}:/var/lib/postgresql/data
    ports:
      - 5432:5432
    restart: unless-stopped

  pgadmin:
    image: dpage/pgadmin4:latest
    container_name: pgadmin
    environment:
      - PGADMIN_DEFAULT_EMAIL=admin@example.com
      - PGADMIN_DEFAULT_PASSWORD=change_me_password
    ports:
      - 5050:80
    depends_on:
      - postgres
    restart: unless-stopped`,
  },

  // ===== 开发运维 =====
  {
    id: 'gitea',
    name: 'Gitea 自托管 Git 服务',
    category: '开发运维',
    icon: 'wrench',
    color: '#609931',
    description: '轻量级自托管 Git 服务，类似 GitHub 私有版',
    services: ['gitea'],
    content: `services:
  gitea:
    image: gitea/gitea:latest
    container_name: gitea
    environment:
      - USER_UID=1000
      - USER_GID=1000
      - TZ=Asia/Shanghai
    volumes:
      - ${D('gitea/data')}:/data
      - /etc/timezone:/etc/timezone:ro
      - /etc/localtime:/etc/localtime:ro
    ports:
      - 3000:3000
      - 2222:22
    restart: unless-stopped`,
  },
  {
    id: 'uptime-kuma',
    name: 'Uptime Kuma 监控面板',
    category: '开发运维',
    icon: 'bar-chart',
    color: '#5cdd8b',
    description: '美观的网站/服务监控工具，支持多种通知方式',
    services: ['uptime-kuma'],
    content: `services:
  uptime-kuma:
    image: louislam/uptime-kuma:1
    container_name: uptime-kuma
    volumes:
      - ${D('uptime-kuma/data')}:/app/data
    ports:
      - 3001:3001
    restart: unless-stopped`,
  },
  {
    id: 'vaultwarden',
    name: 'Vaultwarden 密码管理器',
    category: '开发运维',
    icon: 'lock',
    color: '#7289da',
    description: 'Bitwarden 兼容的轻量密码管理器，自托管安全存储',
    services: ['vaultwarden'],
    content: `services:
  vaultwarden:
    image: vaultwarden/server:latest
    container_name: vaultwarden
    environment:
      - TZ=Asia/Shanghai
      - DOMAIN=https://your-domain.com
    volumes:
      - ${D('vaultwarden/data')}:/data
    ports:
      - 8222:80
    restart: unless-stopped`,
  },

  // ===== 智能家居 =====
  {
    id: 'homeassistant',
    name: 'Home Assistant 智能家居',
    category: '智能家居',
    icon: 'home-city',
    color: '#18bcf2',
    description: '开源智能家居自动化平台，支持数千种设备',
    services: ['homeassistant'],
    content: `services:
  homeassistant:
    image: homeassistant/home-assistant:stable
    container_name: homeassistant
    environment:
      - TZ=Asia/Shanghai
    volumes:
      - ${D('homeassistant/config')}:/config
      - /etc/localtime:/etc/localtime:ro
    network_mode: host
    restart: unless-stopped`,
  },
  {
    id: 'node-red',
    name: 'Node-RED 自动化流程',
    category: '智能家居',
    icon: 'refresh',
    color: '#8f0000',
    description: '可视化编程工具，连接各种硬件和 API 实现自动化',
    services: ['node-red'],
    content: `services:
  nodered:
    image: nodered/node-red:latest
    container_name: nodered
    environment:
      - TZ=Asia/Shanghai
    volumes:
      - ${D('node-red/data')}:/data
    ports:
      - 1880:1880
    restart: unless-stopped`,
  },

  // ===== 建站 =====
  {
    id: 'nginx-php',
    name: 'Nginx + PHP 网站环境',
    category: '建站',
    icon: 'globe',
    color: '#009639',
    description: 'Nginx Web 服务器 + PHP-FPM，经典建站运行环境',
    services: ['nginx', 'php'],
    content: `services:
  nginx:
    image: nginx:alpine
    container_name: nginx
    volumes:
      - ${D('nginx/html')}:/usr/share/nginx/html
      - ${D('nginx/conf')}:/etc/nginx/conf.d
    ports:
      - 8088:80
    restart: unless-stopped

  php:
    image: php:8-fpm-alpine
    container_name: php-fpm
    volumes:
      - ${D('nginx/html')}:/usr/share/nginx/html
    restart: unless-stopped`,
  },
  {
    id: 'wordpress',
    name: 'WordPress 博客系统',
    category: '建站',
    icon: 'edit',
    color: '#21759b',
    description: 'WordPress + MySQL，一键搭建个人博客或企业网站',
    services: ['wordpress', 'mysql'],
    content: `services:
  wordpress:
    image: wordpress:latest
    container_name: wordpress
    environment:
      - WORDPRESS_DB_HOST=db
      - WORDPRESS_DB_USER=wordpress
      - WORDPRESS_DB_PASSWORD=change_me_password
      - WORDPRESS_DB_NAME=wordpress
    volumes:
      - ${D('wordpress/html')}:/var/www/html
    ports:
      - 8082:80
    depends_on:
      - db
    restart: unless-stopped

  db:
    image: mysql:8
    container_name: wordpress-db
    environment:
      - MYSQL_ROOT_PASSWORD=change_me_root_password
      - MYSQL_DATABASE=wordpress
      - MYSQL_USER=wordpress
      - MYSQL_PASSWORD=change_me_password
    volumes:
      - ${D('wordpress/db')}:/var/lib/mysql
    restart: unless-stopped`,
  },
  {
    id: 'nextcloud',
    name: 'NextCloud 私有云盘',
    category: '建站',
    icon: 'cloud',
    color: '#0062cc',
    description: '自托管文件同步共享平台，替代百度网盘/Google Drive',
    services: ['nextcloud'],
    content: `services:
  nextcloud:
    image: nextcloud:latest
    container_name: nextcloud
    environment:
      - TZ=Asia/Shanghai
    volumes:
      - ${D('nextcloud/html')}:/var/www/html
      - ${D('nextcloud/apps')}:/var/www/html/custom_apps
      - ${D('nextcloud/config')}:/var/www/html/config
      - ${D('nextcloud/data')}:/var/www/html/data
    ports:
      - 8083:80
    restart: unless-stopped`,
  },
  {
    id: 'tanqu-compose',
    name: '好学探趣（儿童短视频）',
    category: '媒体中心',
    icon: 'user',
    color: '#f43f5e',
    description: 'NAS 儿童版刷视频软件，精选适合儿童的短视频和影视，自带健康检查',
    services: ['tanqu'],
    content: [
      'services:',
      '  tanqu:',
      '    image: ghcr.io/gntv456/tanqu:latest',
      '    container_name: haoxue-tanqu',
      '    hostname: haoxue-tanqu',
      '    restart: unless-stopped',
      '    pull_policy: always',
      '    ports:',
      '      - "8787:8787"',
      '    environment:',
      '      HOST: "0.0.0.0"',
      '      PORT: "8787"',
      '      TANQU_DATA_DIR: /data',
      '      TANQU_AUTH_DISABLED: "0"',
      '      PUID: "0"',
      '      PGID: "0"',
      '      UMASK: "000"',
      '      TZ: Asia/Shanghai',
      '      HXPT_SITE_URL: https://www.hxpt.org',
      '      PTANG_SITE_URL: https://ptang.top',
      '      TANQU_MEDIA_MOUNTS: >-',
      '        movies=/media/movies,tv=/media/tv,shorts=/media/shorts,drama=/media/drama,video=/video',
      '    volumes:',
      '      - /opt/docker/tanqu/data:/data',
      '      # 按 NAS 实际视频目录修改以下挂载:',
      '      - /opt/docker/videos:/video',
      '      - /opt/docker/movies:/media/movies:ro',
      '      - /opt/docker/tv:/media/tv:ro',
      '      - /opt/docker/shorts:/media/shorts:ro',
      '    healthcheck:',
      '      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8787/api/health"]',
      '      interval: 30s',
      '      timeout: 5s',
      '      retries: 3',
      '      start_period: 25s',
      '    logging:',
      '      driver: json-file',
      '      options:',
      '        max-size: "10m"',
      '        max-file: "3"',
    ].join('\n'),
  },
]
