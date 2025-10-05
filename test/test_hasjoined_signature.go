package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "http://localhost:8080"

type AuthResponse struct {
	AccessToken       string `json:"accessToken"`
	ClientToken       string `json:"clientToken"`
	AvailableProfiles []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"availableProfiles"`
	SelectedProfile struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"selectedProfile"`
}

type ProfileResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Properties []struct {
		Name      string `json:"name"`
		Value     string `json:"value"`
		Signature string `json:"signature,omitempty"`
	} `json:"properties"`
}

func main() {
	fmt.Println("=== HasJoined 签名测试 ===\n")

	// 1. 登录获取令牌
	fmt.Println("1. 登录获取令牌...")
	authReq := map[string]interface{}{
		"username": "test1@example.com",
		"password": "password123",
		"agent": map[string]interface{}{
			"name":    "Minecraft",
			"version": 1,
		},
	}

	authBody, _ := json.Marshal(authReq)
	resp, err := http.Post(baseURL+"/authserver/authenticate", "application/json", bytes.NewBuffer(authBody))
	if err != nil {
		fmt.Printf("❌ 登录失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ 登录失败 (状态码 %d): %s\n", resp.StatusCode, string(body))
		return
	}

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		fmt.Printf("❌ 解析登录响应失败: %v\n", err)
		return
	}

	fmt.Printf("✅ 登录成功\n")
	fmt.Printf("   AccessToken: %s...\n", authResp.AccessToken[:20])
	fmt.Printf("   角色: %s (UUID: %s)\n\n", authResp.SelectedProfile.Name, authResp.SelectedProfile.ID)

	// 2. 客户端进入服务器
	fmt.Println("2. 客户端进入服务器...")
	serverID := fmt.Sprintf("test-server-%d", time.Now().Unix())
	joinReq := map[string]interface{}{
		"accessToken":     authResp.AccessToken,
		"selectedProfile": authResp.SelectedProfile.ID,
		"serverId":        serverID,
	}

	joinBody, _ := json.Marshal(joinReq)
	resp, err = http.Post(baseURL+"/sessionserver/session/minecraft/join", "application/json", bytes.NewBuffer(joinBody))
	if err != nil {
		fmt.Printf("❌ 进入服务器失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ 进入服务器失败 (状态码 %d): %s\n", resp.StatusCode, string(body))
		return
	}

	fmt.Printf("✅ 客户端成功进入服务器\n")
	fmt.Printf("   ServerID: %s\n\n", serverID)

	// 3. 服务端验证客户端 (hasJoined)
	fmt.Println("3. 服务端验证客户端 (hasJoined)...")
	hasJoinedURL := fmt.Sprintf("%s/sessionserver/session/minecraft/hasJoined?username=%s&serverId=%s",
		baseURL, authResp.SelectedProfile.Name, serverID)

	resp, err = http.Get(hasJoinedURL)
	if err != nil {
		fmt.Printf("❌ hasJoined 请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ hasJoined 失败 (状态码 %d): %s\n", resp.StatusCode, string(body))
		return
	}

	var profile ProfileResponse
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &profile); err != nil {
		fmt.Printf("❌ 解析 hasJoined 响应失败: %v\n", err)
		fmt.Printf("   响应内容: %s\n", string(body))
		return
	}

	fmt.Printf("✅ hasJoined 验证成功\n")
	fmt.Printf("   角色ID: %s\n", profile.ID)
	fmt.Printf("   角色名: %s\n", profile.Name)
	fmt.Printf("   属性数量: %d\n\n", len(profile.Properties))

	// 4. 检查签名
	fmt.Println("4. 检查属性签名...")
	hasSignature := false
	for _, prop := range profile.Properties {
		fmt.Printf("   属性: %s\n", prop.Name)
		fmt.Printf("   值长度: %d 字符\n", len(prop.Value))
		if prop.Signature != "" {
			fmt.Printf("   ✅ 签名: %s...\n", prop.Signature[:50])
			hasSignature = true
		} else {
			fmt.Printf("   ❌ 签名: 无\n")
		}
		fmt.Println()
	}

	// 5. 对比 profile API 的签名
	fmt.Println("5. 对比 /profile/{uuid} API 的签名...")
	profileURL := fmt.Sprintf("%s/sessionserver/session/minecraft/profile/%s?unsigned=false",
		baseURL, profile.ID)

	resp, err = http.Get(profileURL)
	if err != nil {
		fmt.Printf("❌ profile API 请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ profile API 失败 (状态码 %d): %s\n", resp.StatusCode, string(body))
		return
	}

	var profileFromAPI ProfileResponse
	body, _ = io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &profileFromAPI); err != nil {
		fmt.Printf("❌ 解析 profile API 响应失败: %v\n", err)
		return
	}

	fmt.Printf("✅ profile API 调用成功\n")
	fmt.Printf("   属性数量: %d\n\n", len(profileFromAPI.Properties))

	// 比较签名
	fmt.Println("6. 签名对比结果...")
	if len(profile.Properties) != len(profileFromAPI.Properties) {
		fmt.Printf("⚠️  属性数量不一致: hasJoined=%d, profile=%d\n",
			len(profile.Properties), len(profileFromAPI.Properties))
	}

	allMatch := true
	for i := range profile.Properties {
		if i >= len(profileFromAPI.Properties) {
			break
		}

		hasJoinedProp := profile.Properties[i]
		profileProp := profileFromAPI.Properties[i]

		if hasJoinedProp.Name != profileProp.Name {
			fmt.Printf("❌ 属性名不匹配: %s vs %s\n", hasJoinedProp.Name, profileProp.Name)
			allMatch = false
			continue
		}

		if hasJoinedProp.Signature == "" && profileProp.Signature != "" {
			fmt.Printf("❌ %s: hasJoined 缺少签名\n", hasJoinedProp.Name)
			allMatch = false
		} else if hasJoinedProp.Signature != "" && profileProp.Signature == "" {
			fmt.Printf("⚠️  %s: profile API 缺少签名\n", hasJoinedProp.Name)
		} else if hasJoinedProp.Signature == profileProp.Signature {
			fmt.Printf("✅ %s: 签名一致\n", hasJoinedProp.Name)
		} else if hasJoinedProp.Signature != "" && profileProp.Signature != "" {
			fmt.Printf("⚠️  %s: 签名不同（可能是时间戳导致）\n", hasJoinedProp.Name)
			fmt.Printf("   hasJoined: %s...\n", hasJoinedProp.Signature[:30])
			fmt.Printf("   profile:   %s...\n", profileProp.Signature[:30])
		}
	}

	// 总结
	fmt.Println("\n=== 测试总结 ===")
	if hasSignature && allMatch {
		fmt.Println("✅ 所有测试通过！hasJoined API 正确返回了带签名的角色信息")
	} else if hasSignature {
		fmt.Println("⚠️  hasJoined API 返回了签名，但与 profile API 存在差异")
	} else {
		fmt.Println("❌ hasJoined API 未返回签名，需要修复")
	}
}
