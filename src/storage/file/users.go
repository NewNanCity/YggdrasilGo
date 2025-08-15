// Package file 文件存储用户管理
package file

import (
	"fmt"
	"os"
	"path/filepath"

	"yggdrasil-api-go/src/yggdrasil"

	"github.com/bytedance/sonic"
)

// loadUsers 加载用户数据
func (s *Storage) loadUsers() error {
	usersFile := filepath.Join(s.dataDir, "users.json")

	// 如果文件不存在，创建默认数据
	if _, err := os.Stat(usersFile); os.IsNotExist(err) {
		return s.createDefaultUsers()
	}

	data, err := os.ReadFile(usersFile)
	if err != nil {
		return err
	}

	var users []*FileUser
	if err := sonic.Unmarshal(data, &users); err != nil {
		return err
	}

	// 加载到缓存
	for _, user := range users {
		s.users[user.Email] = user
		s.userProfiles[user.Email] = make([]string, 0)
	}

	return nil
}

// saveUsers 保存用户数据
func (s *Storage) saveUsers() error {
	var users []*FileUser
	for _, user := range s.users {
		users = append(users, user)
	}

	data, err := sonic.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}

	usersFile := filepath.Join(s.dataDir, "users.json")
	return os.WriteFile(usersFile, data, 0644)
}

// createDefaultUsers 创建默认用户数据
func (s *Storage) createDefaultUsers() error {
	// 创建测试用户
	testUsers := []*FileUser{
		{
			UID:        1,
			Email:      "test@example.com",
			Password:   "$2a$10$example1", // 实际应该是bcrypt哈希
			Nickname:   "测试用户",
			Score:      1000,
			Permission: 0,
			Verified:   true,
			RegisterAt: "2024-01-01 00:00:00",
			LastSignAt: "2024-01-01 00:00:00",
		},
		{
			UID:        2,
			Email:      "user2@example.com",
			Password:   "$2a$10$example2",
			Nickname:   "用户2",
			Score:      1000,
			Permission: 0,
			Verified:   true,
			RegisterAt: "2024-01-01 00:00:00",
			LastSignAt: "2024-01-01 00:00:00",
		},
		{
			UID:        3,
			Email:      "admin@example.com",
			Password:   "$2a$10$example3",
			Nickname:   "管理员",
			Score:      1000,
			Permission: 1,
			Verified:   true,
			RegisterAt: "2024-01-01 00:00:00",
			LastSignAt: "2024-01-01 00:00:00",
		},
	}

	for _, user := range testUsers {
		s.users[user.Email] = user
		s.userProfiles[user.Email] = make([]string, 0)
	}

	return s.saveUsers()
}

// GetUserByEmail 根据邮箱获取用户
func (s *Storage) GetUserByEmail(email string) (*yggdrasil.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if user, exists := s.users[email]; exists {
		return s.convertFileUserToYggdrasilUser(user)
	}

	return nil, fmt.Errorf("user not found")
}

// GetUserByPlayerName 根据角色名获取用户
func (s *Storage) GetUserByPlayerName(playerName string) (*yggdrasil.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 先通过角色名找到对应的角色
	var targetPlayer *FilePlayer
	for _, player := range s.players {
		if player.Name == playerName {
			targetPlayer = player
			break
		}
	}

	if targetPlayer == nil {
		return nil, fmt.Errorf("player not found")
	}

	// 通过角色的UID找到对应的用户
	for _, user := range s.users {
		if user.UID == targetPlayer.UID {
			return s.convertFileUserToYggdrasilUser(user)
		}
	}

	return nil, fmt.Errorf("user not found")
}

// GetUserByID 根据用户ID获取用户
func (s *Storage) GetUserByID(userID string) (*yggdrasil.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 遍历所有用户，找到匹配的UID
	for _, user := range s.users {
		if fmt.Sprintf("%d", user.UID) == userID {
			return s.convertFileUserToYggdrasilUser(user)
		}
	}

	return nil, fmt.Errorf("user not found")
}

// CreateUser 创建用户
func (s *Storage) CreateUser(user *yggdrasil.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.Email]; exists {
		return fmt.Errorf("user already exists")
	}

	fileUser, err := s.convertYggdrasilUserToFileUser(user)
	if err != nil {
		return err
	}

	s.users[user.Email] = fileUser
	s.userProfiles[user.Email] = make([]string, 0)

	return s.saveUsers()
}

// UpdateUser 更新用户信息
func (s *Storage) UpdateUser(user *yggdrasil.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.Email]; !exists {
		return fmt.Errorf("user not found")
	}

	fileUser, err := s.convertYggdrasilUserToFileUser(user)
	if err != nil {
		return err
	}

	s.users[user.Email] = fileUser
	return s.saveUsers()
}

// DeleteUser 删除用户
func (s *Storage) DeleteUser(email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 获取用户信息
	user, exists := s.users[email]
	if !exists {
		return fmt.Errorf("user not found")
	}

	// 删除用户的所有角色
	for playerID, player := range s.players {
		if player.UID == user.UID {
			delete(s.players, playerID)
		}
	}

	// 删除用户角色映射
	delete(s.userProfiles, email)

	// 删除用户
	delete(s.users, email)

	// 保存数据
	return s.saveUsers()
}

// ListUsers 列出所有用户（分页）
func (s *Storage) ListUsers(offset, limit int) ([]*yggdrasil.User, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var users []*yggdrasil.User
	for _, user := range s.users {
		yggdrasilUser, err := s.convertFileUserToYggdrasilUser(user)
		if err != nil {
			continue // 跳过转换失败的用户
		}
		users = append(users, yggdrasilUser)
	}

	total := len(users)

	// 应用分页
	start := offset
	if start > total {
		start = total
	}

	end := start + limit
	if end > total {
		end = total
	}

	return users[start:end], total, nil
}
