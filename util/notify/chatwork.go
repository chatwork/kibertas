package notify

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

type Chatwork struct {
	ApiToken string
	RoomId   string
	Site     string
	Logger   func() *logrus.Entry
	Messages strings.Builder
}

func NewChatwork(apiToken string, roomId string, logger func() *logrus.Entry) *Chatwork {
	site := "api.chatwork.com"

	if os.Getenv("CHATWORK_SITE") != "" {
		site = os.Getenv("CHATWORK_SITE")
	}
	return &Chatwork{
		ApiToken: apiToken,
		RoomId:   roomId,
		Logger:   logger,
		Site:     site,
	}
}

func (c *Chatwork) AddMessage(message string) {
	if _, err := c.Messages.WriteString(message); err != nil {
		c.Logger().Error(err)
	}
}

// Send メッセージを送信する
// https://developer.chatwork.com/ja/endpoint_rooms.html#POST-rooms-room_id-messages
// エラーが起きても問題ないので、エラーはログに出力するだけ
func (c *Chatwork) Send() {
	defer c.Messages.Reset()
	// APIのURLを作成
	apiUrl := fmt.Sprintf("https://api.chatwork.com/v2/rooms/%s/messages", c.RoomId)

	// メッセージをエンコード
	data := url.Values{}
	data.Set("body", c.Messages.String())

	// リクエストを作成
	req, err := http.NewRequest("POST", apiUrl, bytes.NewBufferString(data.Encode()))
	if err != nil {
		c.Logger().Error(err)
	}

	// ヘッダーを設定
	req.Header.Add("X-ChatWorkToken", c.ApiToken)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// リクエストを送信
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.Logger().Error(err)
	}
	defer resp.Body.Close()

	// ステータスコードを表示
	c.Logger().Infoln("Chatwork Send Message Status:", resp.Status)
}
