package main

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/jinzhu/configor"
	"github.com/mailru/easyjson"
	"github.com/spf13/pflag"

	vkcb "github.com/stek29/vkCallbackApi"
)

var vkClient *vkcb.API
var cyrilRegex = regexp.MustCompile(`(?i)[а-яёй]`)

var confs map[int]GroupConf

type GroupConf struct {
	GroupID      int
	Secret       string
	Confirmation string
}

type extConfig struct {
	VKToken string
	Groups  []GroupConf
}

func getNameForUID(userID int) (string, error) {
	users, err := vkcb.APIUsers{vkClient}.Get(vkcb.UsersGetParams{
		UserIDs: []string{strconv.Itoa(userID)},
	})

	if err != nil {
		return "", err
	}

	if len(users) < 1 || users[0].ID != userID {
		return "", errors.New("VK didn't return user we wanted")
	}

	return users[0].FirstName, nil
}

func getNameForGID(groupID int) (string, error) {
	groups, err := vkcb.APIGroups{vkClient}.GetByID(vkcb.GroupsGetByIDParams{
		GroupID: strconv.Itoa(groupID),
	})

	if err != nil {
		return "", err
	}

	if len(groups) < 1 || groups[0].ID != groupID {
		return "", errors.New("VK didn't return group we wanted")
	}

	return groups[0].Name, nil
}

func getNameForID(ID int) (string, error) {
	switch {
	case ID > 0:
		return getNameForUID(ID)
	case ID < 0:
		return getNameForGID(-ID)
	default:
		return "", errors.New("ID cant be 0!")
	}
}

func handleComment(ownerID int, comment vkcb.Comment) {
	commentText := comment.Text

	if replyTo := comment.ReplyToUser; replyTo != 0 {
		name, err := getNameForID(replyTo)

		if err != nil {
			log.Printf("Cant get first name for id%d: %v", replyTo, err)
		}

		commentText = strings.Replace(commentText, name, "", -1)
	}

	if cyrilRegex.MatchString(commentText) {
		ok, err := vkcb.APIWall{vkClient}.DeleteComment(vkcb.WallDeleteCommentParams{
			OwnerID:   ownerID,
			CommentID: comment.ID,
		})

		if err != nil || ok != true {
			log.Printf("Cant delete comment %d_%d: %v", ownerID, comment.ID, err)
		}

		log.Printf("Deleted Comment: %d_%d", ownerID, comment.ID)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	event := vkcb.CallbackEvent{}
	if err := easyjson.UnmarshalFromReader(r.Body, &event); err != nil {
		log.Printf("Cant unmarshal event: %v", err)
		return
	}

	log.Printf("Got event: GroupID=%v Secret=%v Event=%T", event.GroupID, event.Secret, event.Event)

	var conf GroupConf
	var ok bool

	if conf, ok = confs[event.GroupID]; !ok {
		log.Printf("There's no VKSourceConf for Group %d, dropping", event.GroupID)
		return
	}

	if conf.Secret != event.Secret {
		log.Printf("Secret mismatch, dropping")
		return
	}

	if _, ok := event.Event.(vkcb.Confirmation); ok {
		w.Write([]byte(conf.Confirmation))
		return
	}

	defer w.Write([]byte("ok\n"))

	switch v := event.Event.(type) {
	case vkcb.WallReplyNew:
		go handleComment(v.PostOwnerID, v.Comment)
	case vkcb.WallReplyEdit:
		go handleComment(v.PostOwnerID, v.Comment)
	case vkcb.WallReplyRestore:
		go handleComment(v.PostOwnerID, v.Comment)
	case vkcb.PhotoCommentNew:
		go handleComment(v.PhotoOwnerID, v.Comment)
	case vkcb.PhotoCommentEdit:
		go handleComment(v.PhotoOwnerID, v.Comment)
	case vkcb.PhotoCommentRestore:
		go handleComment(v.PhotoOwnerID, v.Comment)
	case vkcb.VideoCommentNew:
		go handleComment(v.VideoOwnerID, v.Comment)
	case vkcb.VideoCommentEdit:
		go handleComment(v.VideoOwnerID, v.Comment)
	case vkcb.VideoCommentRestore:
		go handleComment(v.VideoOwnerID, v.Comment)
	case vkcb.MarketCommentNew:
		go handleComment(v.MarketOwnerID, v.Comment)
	case vkcb.MarketCommentEdit:
		go handleComment(v.MarketOwnerID, v.Comment)
	case vkcb.MarketCommentRestore:
		go handleComment(v.MarketOwnerID, v.Comment)
	}
}

func main() {
	lAddr := pflag.StringP("listen", "l", "127.0.0.1:8081", "host:port to listen")
	confPath := pflag.StringP("conf", "c", "nocyril.yaml", "Path to config file")

	pflag.Parse()

	var config extConfig

	if err := configor.Load(&config, *confPath); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	vkToken := config.VKToken
	if vkToken == "" {
		log.Fatal("vk token is required (check config file)")
	}
	vkClient = vkcb.APIWithAccessToken(vkToken)

	confs = make(map[int]GroupConf, len(config.Groups))
	for _, gc := range config.Groups {
		confs[gc.GroupID] = gc
	}

	log.Print("Starting server")
	http.HandleFunc("/", handler)
	http.ListenAndServe(*lAddr, nil)
}
