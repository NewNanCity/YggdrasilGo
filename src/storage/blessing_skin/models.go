// Package blessing_skin BlessingSkin数据模型定义
package blessing_skin

import (
	"time"
)

// User 用户模型（对应users表）
type User struct {
	UID               uint      `gorm:"primaryKey;column:uid;autoIncrement"`
	Email             string    `gorm:"column:email;size:100;not null"`
	Nickname          string    `gorm:"column:nickname;size:50;not null;default:''"`
	Locale            *string   `gorm:"column:locale;size:255"`
	Score             int       `gorm:"column:score;not null"`
	Avatar            int       `gorm:"column:avatar;not null;default:0"`
	Password          string    `gorm:"column:password;size:255;not null"`
	IP                string    `gorm:"column:ip;size:45;not null"`
	IsDarkMode        bool      `gorm:"column:is_dark_mode;not null;default:false"`
	Permission        int       `gorm:"column:permission;not null;default:0"`
	LastSignAt        time.Time `gorm:"column:last_sign_at;not null"`
	RegisterAt        time.Time `gorm:"column:register_at;not null"`
	Verified          bool      `gorm:"column:verified;not null;default:false"`
	VerificationToken string    `gorm:"column:verification_token;size:255;not null;default:''"`
	RememberToken     *string   `gorm:"column:remember_token;size:100"`
}

func (User) TableName() string {
	return "users"
}

// Player 角色模型（对应players表）
type Player struct {
	PID          uint      `gorm:"primaryKey;column:pid;autoIncrement"`
	UID          int       `gorm:"column:uid;not null"`
	Name         string    `gorm:"column:name;size:50;not null"`
	TIDCape      int       `gorm:"column:tid_cape;not null;default:0"`
	LastModified time.Time `gorm:"column:last_modified;not null"`
	TIDSkin      int       `gorm:"column:tid_skin;not null;default:-1"` // 注意默认值是-1

	// 关联（不会创建外键）
	User User      `gorm:"foreignKey:UID;references:UID"`
	Skin *Texture  `gorm:"foreignKey:TIDSkin;references:TID"`
	Cape *Texture  `gorm:"foreignKey:TIDCape;references:TID"`
}

func (Player) TableName() string {
	return "players"
}

// Texture 材质模型（对应textures表）
type Texture struct {
	TID      uint      `gorm:"primaryKey;column:tid;autoIncrement"`
	Name     string    `gorm:"column:name;size:50;not null"`
	Type     string    `gorm:"column:type;size:10;not null"` // steve, alex, cape
	Hash     string    `gorm:"column:hash;size:64;not null"`
	Size     int       `gorm:"column:size;not null"`
	Uploader int       `gorm:"column:uploader;not null"`
	Public   int8      `gorm:"column:public;not null"`
	UploadAt time.Time `gorm:"column:upload_at;not null"`
	Likes    uint      `gorm:"column:likes;not null;default:0"`
}

func (Texture) TableName() string {
	return "textures"
}

// UUIDMapping UUID映射模型（对应uuid表）
type UUIDMapping struct {
	ID   uint   `gorm:"primaryKey;column:id;autoIncrement"`
	Name string `gorm:"column:name;size:255;not null"`
	UUID string `gorm:"column:uuid;size:255;not null"`
}

func (UUIDMapping) TableName() string {
	return "uuid"
}

// Option 配置模型（对应options表）
type Option struct {
	ID          uint   `gorm:"primaryKey;column:id;autoIncrement"`
	OptionName  string `gorm:"column:option_name;size:50;not null"`
	OptionValue string `gorm:"column:option_value;type:longtext;not null"`
}

func (Option) TableName() string {
	return "options"
}

// YggLog Yggdrasil日志模型（对应ygg_log表）
type YggLog struct {
	ID         uint      `gorm:"primaryKey;column:id;autoIncrement"`
	Action     string    `gorm:"column:action;size:255;not null"`
	UserID     int       `gorm:"column:user_id;not null"`
	PlayerID   int       `gorm:"column:player_id;not null"`
	Parameters string    `gorm:"column:parameters;size:255;not null;default:''"`
	IP         string    `gorm:"column:ip;size:255;not null;default:''"`
	Time       time.Time `gorm:"column:time;not null"`
}

func (YggLog) TableName() string {
	return "ygg_log"
}

// MojangVerification Mojang验证模型（对应mojang_verifications表）
type MojangVerification struct {
	ID        uint       `gorm:"primaryKey;column:id;autoIncrement"`
	UserID    int        `gorm:"column:user_id;not null;uniqueIndex"`
	UUID      string     `gorm:"column:uuid;size:32;not null;uniqueIndex"`
	Verified  bool       `gorm:"column:verified;not null"`
	CreatedAt *time.Time `gorm:"column:created_at"`
	UpdatedAt *time.Time `gorm:"column:updated_at"`
}

func (MojangVerification) TableName() string {
	return "mojang_verifications"
}
