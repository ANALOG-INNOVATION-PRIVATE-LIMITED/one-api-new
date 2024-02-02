package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type DiscordOAuthResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}

type DiscordUserGuildsResponse struct {
	Id string `json:"id"`
}

type DiscordGuildMemberResponse struct {
	Roles []string `json:"roles"`
}

type DiscordUser struct {
	Id       string `json:"id"`
	Username string `json:"username"`
}

func getDiscordUserInfoByCode(codeFromURLParamaters string, host string) (*DiscordUser, error) {
	if codeFromURLParamaters == "" {
		return nil, errors.New("无效参数")
	}

	RequestClient := &http.Client{}

	url := "client_id=%s&client_secret=%s&grant_type=authorization_code&redirect_uri=%s/oauth/discord&code=%s&scope=identify"

	if config.DiscordGuildCheckEnabled {
		url = url + "%20guilds"
	}

	if config.DiscordMemberRoleCheckEnabled {
		url = url + "%20guilds.members.read"
	}

	accessTokenBody := bytes.NewBuffer([]byte(fmt.Sprintf(url,
		config.DiscordClientId, config.DiscordClientSecret, config.ServerAddress, codeFromURLParamaters,
	)))

	req, _ := http.NewRequest("POST",
		"https://discordapp.com/api/oauth2/token",
		accessTokenBody,
	)

	req.Header = http.Header{
		"Content-Type": []string{"application/x-www-form-urlencoded"},
		"Accept":       []string{"application/json"},
	}

	resp, err := RequestClient.Do(req)

	if resp.StatusCode != 200 || err != nil {
		return nil, errors.New("访问令牌无效")
	}

	var discordOAuthResponse DiscordOAuthResponse

	err = json.NewDecoder(resp.Body).Decode(&discordOAuthResponse)

	if err != nil {
		return nil, err
	}

	accessToken := fmt.Sprintf("Bearer %s", discordOAuthResponse.AccessToken)

	// Get User Info
	req, _ = http.NewRequest("GET", "https://discord.com/api/users/@me", nil)

	req.Header = http.Header{
		"Content-Type":  []string{"application/json"},
		"Authorization": []string{accessToken},
	}

	defer resp.Body.Close()

	resp, err = RequestClient.Do(req)

	if resp.StatusCode != 200 || err != nil {
		return nil, errors.New("Discord 用户信息无效")
	}

	var discordUser DiscordUser

	err = json.NewDecoder(resp.Body).Decode(&discordUser)

	if err != nil {
		return nil, err
	}

	if discordUser.Id == "" {
		return nil, errors.New("返回值无效，用户字段为空，请稍后再试！")
	}

	defer resp.Body.Close()

	if config.DiscordGuildId != "" && config.DiscordGuildCheckEnabled {
		var discordUserGuildsResponse []DiscordUserGuildsResponse

		// Get User Info
		req, _ = http.NewRequest("GET", "https://discord.com/api/users/@me/guilds", nil)

		req.Header = http.Header{
			"Content-Type":  []string{"application/json"},
			"Authorization": []string{accessToken},
		}

		resp, err = RequestClient.Do(req)

		if resp.StatusCode != 200 || err != nil {
			return nil, errors.New("Invalid Discord User Guild Info!")
		}

		err = json.NewDecoder(resp.Body).Decode(&discordUserGuildsResponse)

		if err != nil {
			return nil, err
		}

		if len(discordUserGuildsResponse) == 0 {
			return nil, errors.New("You are not in the Discord server!")
		}

		joinedDiscordServer := false

		for _, guild := range discordUserGuildsResponse {
			if guild.Id == config.DiscordGuildId {
				joinedDiscordServer = true
				break
			}
		}

		if !joinedDiscordServer {
			return nil, errors.New("You are not in the Discord server!")
		}

		if config.DiscordMemberRoleId != "" && config.DiscordMemberRoleCheckEnabled {
			// /users/@me/guilds/{guild.id}/member
			req, _ = http.NewRequest("GET", fmt.Sprintf("https://discord.com/api/users/@me/guilds/%s/member", config.DiscordGuildId), nil)

			req.Header = http.Header{
				"Content-Type":  []string{"application/json"},
				"Authorization": []string{accessToken},
			}

			resp, err = RequestClient.Do(req)

			if resp.StatusCode != 200 || err != nil {
				return nil, errors.New("Invalid Discord User Guild Member Info!")
			}

			var discordUserGuildMemberResponse DiscordGuildMemberResponse

			err = json.NewDecoder(resp.Body).Decode(&discordUserGuildMemberResponse)

			if err != nil {
				return nil, err
			}

			if len(discordUserGuildMemberResponse.Roles) == 0 {
				return nil, errors.New("You do not have the required role to join the Discord server!")
			}

			isMember := false

			for _, role := range discordUserGuildMemberResponse.Roles {
				if role == config.DiscordMemberRoleId {
					isMember = true
					break
				}
			}

			if !isMember {
				return nil, errors.New("You do not have the required role to join the Discord server!")
			}
		}
	}

	return &discordUser, nil
}

func DiscordOAuth(c *gin.Context) {
	session := sessions.Default(c)
	username := session.Get("username")
	if username != nil {
		DiscordBind(c)
		return
	}

	if !config.DiscordOAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 Discord 登录以及注册",
		})
		return
	}
	code := c.Query("code")

	discordUser, err := getDiscordUserInfoByCode(code, c.Request.Host)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user := model.User{
		DiscordId: discordUser.Id,
	}
	if model.IsDiscordIdAlreadyTaken(user.DiscordId) {
		err := user.FillUserByDiscordId()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	} else {
		if config.RegisterEnabled {
			user.Username = "discord_" + strconv.Itoa(model.GetMaxUserId()+1)
			if discordUser.Username != "" {
				user.DisplayName = discordUser.Username
			} else {
				user.DisplayName = "Discord User"
			}
			user.Role = common.RoleCommonUser
			user.Status = common.UserStatusEnabled

			if err := user.Insert(0); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "管理员关闭了新用户注册",
			})
			return
		}
	}

	if user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "用户已被封禁",
			"success": false,
		})
		return
	}
	setupLogin(&user, c)
}

func DiscordBind(c *gin.Context) {
	if !config.DiscordOAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 Discord 登录以及注册",
		})
		return
	}
	code := c.Query("code")

	discordUser, err := getDiscordUserInfoByCode(code, c.Request.Host)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user := model.User{
		DiscordId: discordUser.Id,
	}
	if model.IsDiscordIdAlreadyTaken(user.DiscordId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该 Discord 账户已被绑定",
		})
		return
	}
	session := sessions.Default(c)
	id := session.Get("id")
	// id := c.GetInt("id")  // critical bug!
	user.Id = id.(int)
	err = user.FillUserById()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user.DiscordId = discordUser.Id
	err = user.Update(false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "bind",
	})
}
