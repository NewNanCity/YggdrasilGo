让我查看reference目录中的PHP插件源码来分析BlessingSkin的Yggdrasil实现。

现在让我查看一些关键的控制器来了解数据库结构和业务逻辑：

现在让我查看BlessingSkin的核心数据库表结构。基于我看到的代码，我来制定完整的兼容实现文档：

# BlessingSkin Yggdrasil API 兼容实现文档

## ⚠️ 重要警告
**绝对不允许GORM自动创建或修改表结构！必须使用 `gorm.Config{DisableForeignKeyConstraintWhenMigrating: true}` 并设置 `AutoMigrate: false`，以防止对生产环境造成破坏。**

## 1. 数据库表结构分析（基于实际DDL）

### 1.1 BlessingSkin核心表（已存在，不可修改）

#### users表
```sql
CREATE TABLE `users` (
  `uid` int unsigned NOT NULL AUTO_INCREMENT,
  `email` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `nickname` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `locale` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `score` int NOT NULL,
  `avatar` int NOT NULL DEFAULT '0',
  `password` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `ip` varchar(45) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `is_dark_mode` tinyint(1) NOT NULL DEFAULT '0',
  `permission` int NOT NULL DEFAULT '0',  -- 0=normal, 1=banned, 2=admin
  `last_sign_at` datetime NOT NULL,
  `register_at` datetime NOT NULL,
  `verified` tinyint(1) NOT NULL DEFAULT '0',
  `verification_token` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `remember_token` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
```

#### players表
```sql
CREATE TABLE `players` (
  `pid` int unsigned NOT NULL AUTO_INCREMENT,
  `uid` int NOT NULL,  -- 关联users.uid（无外键约束）
  `name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `tid_cape` int NOT NULL DEFAULT '0',  -- 关联textures.tid
  `last_modified` datetime NOT NULL,
  `tid_skin` int NOT NULL DEFAULT '-1', -- 关联textures.tid，注意默认值是-1
  PRIMARY KEY (`pid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
```

#### textures表
```sql
CREATE TABLE `textures` (
  `tid` int unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `type` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL, -- steve, alex, cape
  `hash` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `size` int NOT NULL,
  `uploader` int NOT NULL,  -- 关联users.uid
  `public` tinyint NOT NULL,
  `upload_at` datetime NOT NULL,
  `likes` int unsigned NOT NULL DEFAULT '0',
  PRIMARY KEY (`tid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
```

### 1.2 Yggdrasil插件表（由插件创建）

#### uuid表
```sql
CREATE TABLE `uuid` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `uuid` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
```

#### ygg_log表
```sql
CREATE TABLE `ygg_log` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `action` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` int NOT NULL,
  `player_id` int NOT NULL,
  `parameters` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `ip` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `time` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
```

#### mojang_verifications表
```sql
CREATE TABLE `mojang_verifications` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `uuid` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `verified` tinyint(1) NOT NULL,
  `created_at` timestamp NULL DEFAULT NULL,
  `updated_at` timestamp NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `mojang_verifications_user_id_unique` (`user_id`),
  UNIQUE KEY `mojang_verifications_uuid_unique` (`uuid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
```

#### options表
```sql
CREATE TABLE `options` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `option_name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `option_value` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
```

### 2.1 Yggdrasil相关配置项
```
ygg_uuid_algorithm: 'v3' | 'v4'           -- UUID生成算法
ygg_token_expire_1: '259200'              -- 访问令牌过期时间（秒）
ygg_token_expire_2: '604800'              -- 刷新令牌过期时间（秒）
ygg_tokens_limit: '10'                    -- 每用户最大令牌数
ygg_rate_limit: '1000'                    -- 速率限制
ygg_skin_domain: ''                       -- 皮肤域名白名单
ygg_search_profile_max: '5'               -- 批量查询角色最大数量
ygg_private_key: ''                       -- RSA私钥（PEM格式）
ygg_show_config_section: 'true'           -- 显示配置面板
ygg_show_activities_section: 'true'       -- 显示活动面板
ygg_enable_ali: 'true'                    -- 启用ALI头
jwt_secret: ''                            -- JWT密钥
```

## 3. 数据存储策略

### 3.1 Token存储
- **存储方式**: Laravel Cache（Redis/Memcached/File）
- **缓存键**: `yggdrasil-token-{accessToken}`
- **用户令牌列表**: `yggdrasil-id-{email}`
- **过期策略**: 自动过期 + 定期清理

### 3.2 Session存储
- **存储方式**: Laravel Cache
- **缓存键**: `yggdrasil-server-{serverId}`
- **过期时间**: 120秒
- **数据结构**: `{profile: uuid, ip: clientIP}`

### 3.3 材质存储
- **存储方式**: 文件系统或对象存储
- **路径**: `storage/textures/{hash}`
- **URL**: `{site_url}/textures/{hash}`

## 4. UUID生成算法

### 4.1 算法选择
```php
// v3算法（兼容离线模式）
function generateUuidV3(string $name): string {
    $data = hex2bin(md5('OfflinePlayer:' . $name));
    $data[6] = chr(ord($data[6]) & 0x0F | 0x30);
    $data[8] = chr(ord($data[8]) & 0x3F | 0x80);
    return bin2hex($data);
}

// v4算法（随机生成）
$uuid = Uuid::uuid4()->getHex()->toString();
```

### 4.2 UUID管理
- 首次查询角色时生成UUID并存储到`uuid`表
- 角色改名时保持UUID不变（仅v4算法）
- UUID格式：32位无连字符（如：`550e8400e29b41d4a716446655440000`）

## 5. JWT Token实现

### 5.1 Token结构
```php
// JWT Claims
{
    "iss": "Yggdrasil-Auth",           // 签发者
    "sub": "{userUuid}",               // 用户UUID（基于邮箱生成）
    "yggt": "{randomUuid}",            // 随机令牌ID
    "spr": "{selectedProfileId}",      // 选中的角色ID（可选）
    "iat": timestamp,                  // 签发时间
    "exp": timestamp                   // 过期时间
}
```

### 5.2 用户UUID生成
```php
$userUuid = Uuid::uuid5(Uuid::NAMESPACE_DNS, $email)->getHex()->toString();
```

## 6. API端点映射

### 6.1 认证服务器
```
POST /api/yggdrasil/authserver/authenticate
POST /api/yggdrasil/authserver/refresh
POST /api/yggdrasil/authserver/validate
POST /api/yggdrasil/authserver/invalidate
POST /api/yggdrasil/authserver/signout
```

### 6.2 会话服务器
```
POST /api/yggdrasil/sessionserver/session/minecraft/join
GET  /api/yggdrasil/sessionserver/session/minecraft/hasJoined
GET  /api/yggdrasil/sessionserver/session/minecraft/profile/{uuid}
```

### 6.3 API服务器
```
POST /api/yggdrasil/api/profiles/minecraft
GET  /api/yggdrasil/api/users/profiles/minecraft/{username}
PUT  /api/yggdrasil/api/user/profile/{uuid}/{type}
DELETE /api/yggdrasil/api/user/profile/{uuid}/{type}
```

### 6.4 元数据
```
GET /api/yggdrasil/
```

## 7. 材质签名实现

### 7.1 材质数据结构
```json
{
    "timestamp": 1640995200000,
    "profileId": "550e8400e29b41d4a716446655440000",
    "profileName": "TestPlayer",
    "isPublic": true,
    "textures": {
        "SKIN": {
            "url": "http://example.com/textures/abc123",
            "metadata": {"model": "slim"}
        },
        "CAPE": {
            "url": "http://example.com/textures/def456"
        }
    }
}
```

### 7.2 签名流程
```php
// 1. 生成材质数据JSON
$texturesJson = json_encode($textures, JSON_UNESCAPED_SLASHES | JSON_FORCE_OBJECT);

// 2. Base64编码
$texturesValue = base64_encode($texturesJson);

// 3. RSA签名
openssl_sign($texturesValue, $signature, $privateKey);
$signatureValue = base64_encode($signature);

// 4. 构建properties
$properties = [
    [
        'name' => 'textures',
        'value' => $texturesValue,
        'signature' => $signatureValue
    ],
    [
        'name' => 'uploadableTextures',
        'value' => 'skin,cape'
    ]
];
```

## 8. Go实现要点

### 8.1 数据库连接
```go
// 使用GORM连接BlessingSkin数据库
type BlessingSkinStorage struct {
    db    *gorm.DB
    cache *redis.Client
    config *BlessingSkinConfig
}

type BlessingSkinConfig struct {
    DatabaseDSN string
    RedisURL    string
    SiteURL     string
    TextureDir  string
}
```

### 8.2 模型定义
```go
// 用户模型（对应BlessingSkin users表）
type User struct {
    UID          int       `gorm:"primaryKey;column:uid"`
    Email        string    `gorm:"unique;column:email"`
    Password     string    `gorm:"column:password"`
    Nickname     string    `gorm:"column:nickname"`
    Score        int       `gorm:"default:1000;column:score"`
    Avatar       int       `gorm:"default:0;column:avatar"`
    Permission   int       `gorm:"default:0;column:permission"` // 0=normal, 1=banned, 2=admin
    IP           string    `gorm:"column:ip"`
    LastSignAt   time.Time `gorm:"column:last_sign_at"`
    RegisterAt   time.Time `gorm:"column:register_at"`
    Locale       string    `gorm:"column:locale"`
}

// 角色模型（对应BlessingSkin players表）
type Player struct {
    PID          int       `gorm:"primaryKey;column:pid"`
    UID          int       `gorm:"column:uid"`
    Name         string    `gorm:"unique;column:name"`
    TIDSkin      int       `gorm:"default:0;column:tid_skin"`
    TIDCape      int       `gorm:"default:0;column:tid_cape"`
    LastModified time.Time `gorm:"column:last_modified"`

    // 关联
    User User `gorm:"foreignKey:UID;references:UID"`
    Skin *Texture `gorm:"foreignKey:TIDSkin;references:TID"`
    Cape *Texture `gorm:"foreignKey:TIDCape;references:TID"`
}

// 材质模型（对应BlessingSkin textures表）
type Texture struct {
    TID      int       `gorm:"primaryKey;column:tid"`
    Name     string    `gorm:"column:name"`
    Type     string    `gorm:"column:type"` // steve, alex, cape
    Hash     string    `gorm:"unique;column:hash"`
    Size     int       `gorm:"column:size"`
    Uploader int       `gorm:"column:uploader"`
    Public   int       `gorm:"default:0;column:public"`
    UploadAt time.Time `gorm:"column:upload_at"`
}

// UUID映射模型（对应Yggdrasil uuid表）
type UUIDMapping struct {
    ID   int    `gorm:"primaryKey;column:id"`
    Name string `gorm:"column:name"`
    UUID string `gorm:"column:uuid"`
}

// 配置模型（对应BlessingSkin options表）
type Option struct {
    ID          int    `gorm:"primaryKey;column:id"`
    OptionName  string `gorm:"unique;column:option_name"`
    OptionValue string `gorm:"column:option_value"`
}

// 日志模型（对应Yggdrasil ygg_log表）
type YggLog struct {
    ID         int       `gorm:"primaryKey;column:id"`
    Action     string    `gorm:"column:action"`
    UserID     int       `gorm:"column:user_id"`
    PlayerID   int       `gorm:"column:player_id"`
    Parameters string    `gorm:"column:parameters"`
    IP         string    `gorm:"column:ip"`
    Time       time.Time `gorm:"column:time"`
}
```

### 8.3 配置管理
```go
// 配置获取
func (s *BlessingSkinStorage) GetOption(name string) (string, error) {
    var option Option
    err := s.db.Where("option_name = ?", name).First(&option).Error
    if err != nil {
        return "", err
    }
    return option.OptionValue, nil
}

// 配置设置
func (s *BlessingSkinStorage) SetOption(name, value string) error {
    return s.db.Save(&Option{
        OptionName:  name,
        OptionValue: value,
    }).Error
}
```

### 8.4 UUID生成
```go
// UUID生成算法
func (s *BlessingSkinStorage) generateUUID(playerName string) (string, error) {
    algorithm, _ := s.GetOption("ygg_uuid_algorithm")

    switch algorithm {
    case "v3":
        return s.generateUUIDV3(playerName), nil
    case "v4":
        return uuid.New().String(), nil
    default:
        return s.generateUUIDV3(playerName), nil
    }
}

func (s *BlessingSkinStorage) generateUUIDV3(name string) string {
    // 实现离线模式UUID生成算法
    data := md5.Sum([]byte("OfflinePlayer:" + name))
    data[6] = (data[6] & 0x0F) | 0x30
    data[8] = (data[8] & 0x3F) | 0x80
    return hex.EncodeToString(data[:])
}
```

### 8.5 Token管理
```go
// Token存储到Redis
func (s *BlessingSkinStorage) StoreToken(token *Token) error {
    // 存储单个token
    tokenKey := fmt.Sprintf("yggdrasil-token-%s", token.AccessToken)
    tokenData, _ := sonic.Marshal(token)
    s.cache.Set(tokenKey, tokenData, time.Duration(token.ExpiresAt.Sub(time.Now())))

    // 更新用户token列表
    userKey := fmt.Sprintf("yggdrasil-id-%s", token.Owner)
    s.cache.SAdd(userKey, token.AccessToken)

    return nil
}
```

## 9. 兼容性保证

### 9.1 数据完全兼容
- 使用相同的数据库表结构
- 使用相同的UUID生成算法
- 使用相同的Token格式和存储方式
- 使用相同的配置选项名称和格式

### 9.2 API完全兼容
- 相同的URL路径和HTTP方法
- 相同的请求/响应格式
- 相同的错误码和错误消息
- 相同的材质签名算法

### 9.3 行为完全兼容
- 相同的认证流程
- 相同的会话管理
- 相同的材质处理
- 相同的权限检查

## 10. 实现优先级

### Phase 1: 核心功能
1. 数据库模型定义
2. 基础CRUD操作
3. UUID生成和管理
4. 配置系统

### Phase 2: 认证系统
1. JWT Token生成和验证
2. 用户认证
3. Token刷新和失效
4. 缓存集成

### Phase 3: 会话和角色
1. 会话管理
2. 角色查询
3. 材质处理
4. RSA签名

### Phase 4: 高级功能
1. 材质上传
2. 日志记录
3. 速率限制
4. Mojang验证支持

这个实现文档确保了Go版本与PHP插件在数据层面的完全兼容，两者可以共享同一个数据库而不会产生冲突。
