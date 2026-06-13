package api

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/gotify/location"
	"github.com/gotify/server/v2/auth"
	"github.com/gotify/server/v2/model"
)

// The MessageDatabase interface for encapsulating database access.
type MessageDatabase interface {
	GetMessagesByApplicationSince(appID uint, limit int, since uint) ([]*model.Message, error)
	GetApplicationByID(id uint) (*model.Application, error)
	GetMessagesByUserSince(userID uint, limit int, since uint) ([]*model.Message, error)
	GetMessagesByUserWithFilter(userID uint, limit int, since uint, filter *model.MessageFilter) ([]*model.Message, error)
	GetMessagesByApplicationWithFilter(appID uint, limit int, since uint, filter *model.MessageFilter) ([]*model.Message, error)
	DeleteMessageByID(id uint) error
	GetMessageByID(id uint) (*model.Message, error)
	DeleteMessagesByUser(userID uint) error
	DeleteMessagesByApplication(applicationID uint) error
	CreateMessage(message *model.Message) error
}

var timeNow = time.Now

// Notifier notifies when a new message was created.
type Notifier interface {
	Notify(userID uint, message *model.MessageExternal)
}

// The MessageAPI provides handlers for managing messages.
type MessageAPI struct {
	DB       MessageDatabase
	Notifier Notifier
}

type messageFilterParams struct {
	Limit         int        `form:"limit" binding:"min=1,max=200"`
	Since         uint       `form:"since" binding:"min=0"`
	ApplicationID *uint      `form:"appid"`
	Priority      *int       `form:"priority"`
	PriorityMin   *int       `form:"priority_min"`
	PriorityMax   *int       `form:"priority_max"`
	Before        *time.Time `form:"before"`
	After         *time.Time `form:"after"`
}

func (p *messageFilterParams) toFilter() *model.MessageFilter {
	return &model.MessageFilter{
		ApplicationID: p.ApplicationID,
		Priority:      p.Priority,
		PriorityMin:   p.PriorityMin,
		PriorityMax:   p.PriorityMax,
		Before:        p.Before,
		After:         p.After,
	}
}

func withMessageFilter(ctx *gin.Context, f func(params *messageFilterParams)) {
	params := &messageFilterParams{Limit: 100}
	if err := ctx.ShouldBindWith(params, binding.Query); err == nil {
		f(params)
	} else {
		ctx.AbortWithError(400, err)
	}
}

// GetMessages returns all messages from a user.
// swagger:operation GET /message message getMessages
//
// Return all messages.
//
//	---
//	produces: [application/json]
//	security: [clientTokenAuthorizationHeader: [], clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
//	parameters:
//	- name: limit
//	  in: query
//	  description: the maximal amount of messages to return
//	  required: false
//	  maximum: 200
//	  minimum: 1
//	  default: 100
//	  type: integer
//	- name: since
//	  in: query
//	  description: return all messages with an ID less than this value
//	  minimum: 0
//	  required: false
//	  type: integer
//	  format: int64
//	responses:
//	  200:
//	    description: Ok
//	    schema:
//	        $ref: "#/definitions/PagedMessages"
//	  400:
//	    description: Bad Request
//	    schema:
//	        $ref: "#/definitions/Error"
//	  401:
//	    description: Unauthorized
//	    schema:
//	        $ref: "#/definitions/Error"
//	  403:
//	    description: Forbidden
//	    schema:
//	        $ref: "#/definitions/Error"
func (a *MessageAPI) GetMessages(ctx *gin.Context) {
	userID := auth.GetUserID(ctx)
	withMessageFilter(ctx, func(params *messageFilterParams) {
		// the +1 is used to check if there are more messages and will be removed on buildWithPaging
		messages, err := a.DB.GetMessagesByUserWithFilter(userID, params.Limit+1, params.Since, params.toFilter())
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}
		ctx.JSON(200, buildWithPagingAndFilter(ctx, params, messages))
	})
}

func buildWithPagingAndFilter(ctx *gin.Context, params *messageFilterParams, messages []*model.Message) *model.PagedMessages {
	next := ""
	since := uint(0)
	useMessages := messages
	if len(messages) > params.Limit {
		useMessages = messages[:len(messages)-1]
		since = useMessages[len(useMessages)-1].ID
		url := location.Get(ctx)
		url.Path = ctx.Request.URL.Path
		query := url.Query()
		query.Add("limit", strconv.Itoa(params.Limit))
		query.Add("since", strconv.FormatUint(uint64(since), 10))

		// Preserve all active filter parameters in the next-page URL
		if params.ApplicationID != nil {
			query.Add("appid", strconv.FormatUint(uint64(*params.ApplicationID), 10))
		}
		if params.Priority != nil {
			query.Add("priority", strconv.Itoa(*params.Priority))
		}
		if params.PriorityMin != nil {
			query.Add("priority_min", strconv.Itoa(*params.PriorityMin))
		}
		if params.PriorityMax != nil {
			query.Add("priority_max", strconv.Itoa(*params.PriorityMax))
		}
		if params.Before != nil {
			query.Add("before", params.Before.Format(time.RFC3339))
		}
		if params.After != nil {
			query.Add("after", params.After.Format(time.RFC3339))
		}

		url.RawQuery = query.Encode()
		next = url.String()
	}
	return &model.PagedMessages{
		Paging:   model.Paging{Size: len(useMessages), Limit: params.Limit, Next: next, Since: since},
		Messages: toExternalMessages(useMessages),
	}
}

// GetMessagesWithApplication returns all messages from a specific application.
// swagger:operation GET /application/{id}/message message getAppMessages
//
// Return all messages from a specific application.
//
//	---
//	produces: [application/json]
//	security: [clientTokenAuthorizationHeader: [], clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
//	parameters:
//	- name: id
//	  in: path
//	  description: the application id
//	  required: true
//	  type: integer
//	  format: int64
//	- name: limit
//	  in: query
//	  description: the maximal amount of messages to return
//	  required: false
//	  maximum: 200
//	  minimum: 1
//	  default: 100
//	  type: integer
//	- name: since
//	  in: query
//	  description: return all messages with an ID less than this value
//	  minimum: 0
//	  required: false
//	  type: integer
//	  format: int64
//	responses:
//	  200:
//	    description: Ok
//	    schema:
//	        $ref: "#/definitions/PagedMessages"
//	  400:
//	    description: Bad Request
//	    schema:
//	        $ref: "#/definitions/Error"
//	  401:
//	    description: Unauthorized
//	    schema:
//	        $ref: "#/definitions/Error"
//	  403:
//	    description: Forbidden
//	    schema:
//	        $ref: "#/definitions/Error"
//	  404:
//	    description: Not Found
//	    schema:
//	        $ref: "#/definitions/Error"
func (a *MessageAPI) GetMessagesWithApplication(ctx *gin.Context) {
	withID(ctx, "id", func(id uint) {
		withMessageFilter(ctx, func(params *messageFilterParams) {
			app, err := a.DB.GetApplicationByID(id)
			if success := successOrAbort(ctx, 500, err); !success {
				return
			}
			if app != nil && app.UserID == auth.GetUserID(ctx) {
				// the +1 is used to check if there are more messages and will be removed on buildWithPaging
				messages, err := a.DB.GetMessagesByApplicationWithFilter(id, params.Limit+1, params.Since, params.toFilter())
				if success := successOrAbort(ctx, 500, err); !success {
					return
				}
				ctx.JSON(200, buildWithPagingAndFilter(ctx, params, messages))
			} else {
				ctx.AbortWithError(404, errors.New("application does not exist"))
			}
		})
	})
}

// DeleteMessages delete all messages from a user.
// swagger:operation DELETE /message message deleteMessages
//
// Delete all messages.
//
//	---
//	produces: [application/json]
//	security: [clientTokenAuthorizationHeader: [], clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
//	responses:
//	  200:
//	    description: Ok
//	  401:
//	    description: Unauthorized
//	    schema:
//	        $ref: "#/definitions/Error"
//	  403:
//	    description: Forbidden
//	    schema:
//	        $ref: "#/definitions/Error"
func (a *MessageAPI) DeleteMessages(ctx *gin.Context) {
	userID := auth.GetUserID(ctx)
	successOrAbort(ctx, 500, a.DB.DeleteMessagesByUser(userID))
}

// DeleteMessageWithApplication deletes all messages from a specific application.
// swagger:operation DELETE /application/{id}/message message deleteAppMessages
//
// Delete all messages from a specific application.
//
//	---
//	produces: [application/json]
//	security: [clientTokenAuthorizationHeader: [], clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
//	parameters:
//	- name: id
//	  in: path
//	  description: the application id
//	  required: true
//	  type: integer
//	  format: int64
//	responses:
//	  200:
//	    description: Ok
//	  400:
//	    description: Bad Request
//	    schema:
//	        $ref: "#/definitions/Error"
//	  401:
//	    description: Unauthorized
//	    schema:
//	        $ref: "#/definitions/Error"
//	  403:
//	    description: Forbidden
//	    schema:
//	        $ref: "#/definitions/Error"
//	  404:
//	    description: Not Found
//	    schema:
//	        $ref: "#/definitions/Error"
func (a *MessageAPI) DeleteMessageWithApplication(ctx *gin.Context) {
	withID(ctx, "id", func(id uint) {
		application, err := a.DB.GetApplicationByID(id)
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}
		if application != nil && application.UserID == auth.GetUserID(ctx) {
			successOrAbort(ctx, 500, a.DB.DeleteMessagesByApplication(id))
		} else {
			ctx.AbortWithError(404, errors.New("application does not exists"))
		}
	})
}

// DeleteMessage deletes a message with an id.
// swagger:operation DELETE /message/{id} message deleteMessage
//
// Deletes a message with an id.
//
//	---
//	produces: [application/json]
//	security: [clientTokenAuthorizationHeader: [], clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
//	parameters:
//	- name: id
//	  in: path
//	  description: the message id
//	  required: true
//	  type: integer
//	  format: int64
//	responses:
//	  200:
//	    description: Ok
//	  400:
//	    description: Bad Request
//	    schema:
//	        $ref: "#/definitions/Error"
//	  401:
//	    description: Unauthorized
//	    schema:
//	        $ref: "#/definitions/Error"
//	  403:
//	    description: Forbidden
//	    schema:
//	        $ref: "#/definitions/Error"
//	  404:
//	    description: Not Found
//	    schema:
//	        $ref: "#/definitions/Error"
func (a *MessageAPI) DeleteMessage(ctx *gin.Context) {
	withID(ctx, "id", func(id uint) {
		msg, err := a.DB.GetMessageByID(id)
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}
		if msg == nil {
			ctx.AbortWithError(404, errors.New("message does not exist"))
			return
		}
		app, err := a.DB.GetApplicationByID(msg.ApplicationID)
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}
		if app != nil && app.UserID == auth.GetUserID(ctx) {
			successOrAbort(ctx, 500, a.DB.DeleteMessageByID(id))
		} else {
			ctx.AbortWithError(404, errors.New("message does not exist"))
		}
	})
}

// CreateMessage creates a message, authentication via application token, client token, or basic auth is required.
// swagger:operation POST /message message createMessage
//
// Create a message.
//
// __NOTE__: When authenticating with a client token or basic auth, the request body
// must include "appid" referencing an application owned by the authenticated user.
// When authenticating with an application token, the application is derived from the
// token and any "appid" in the body is ignored.
//
//	---
//	consumes: [application/json]
//	produces: [application/json]
//	security: [appTokenAuthorizationHeader: [], appTokenHeader: [], appTokenQuery: [], clientTokenAuthorizationHeader: [], clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
//	parameters:
//	- name: body
//	  in: body
//	  description: the message to add
//	  required: true
//	  schema:
//	    $ref: "#/definitions/Message"
//	responses:
//	  200:
//	    description: Ok
//	    schema:
//	      $ref: "#/definitions/Message"
//	  400:
//	    description: Bad Request
//	    schema:
//	        $ref: "#/definitions/Error"
//	  401:
//	    description: Unauthorized
//	    schema:
//	        $ref: "#/definitions/Error"
//	  403:
//	    description: Forbidden
//	    schema:
//	        $ref: "#/definitions/Error"
func (a *MessageAPI) CreateMessage(ctx *gin.Context) {
	message := model.MessageExternal{}
	if err := ctx.Bind(&message); err != nil {
		return
	}

	app := auth.GetApplication(ctx)
	if app == nil {
		if message.ApplicationID == 0 {
			ctx.AbortWithError(400, errors.New("appid is required when not authenticating with an application token"))
			return
		}
		fetchedApp, err := a.DB.GetApplicationByID(message.ApplicationID)
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}
		if fetchedApp == nil || fetchedApp.UserID != auth.GetUserID(ctx) {
			ctx.AbortWithError(400, errors.New("appid not found"))
			return
		}
		app = fetchedApp
	}

	message.ApplicationID = app.ID
	if strings.TrimSpace(message.Title) == "" {
		message.Title = app.Name
	}

	if message.Priority == nil {
		message.Priority = &app.DefaultPriority
	}

	message.Date = timeNow()
	message.ID = 0
	msgInternal := toInternalMessage(&message)
	if success := successOrAbort(ctx, 500, a.DB.CreateMessage(msgInternal)); !success {
		return
	}
	a.Notifier.Notify(auth.GetUserID(ctx), toExternalMessage(msgInternal))
	ctx.JSON(200, toExternalMessage(msgInternal))
}

func toInternalMessage(msg *model.MessageExternal) *model.Message {
	res := &model.Message{
		ID:            msg.ID,
		ApplicationID: msg.ApplicationID,
		Message:       msg.Message,
		Title:         msg.Title,
		Date:          msg.Date,
	}
	if msg.Priority != nil {
		res.Priority = *msg.Priority
	}

	if msg.Extras != nil {
		res.Extras, _ = json.Marshal(msg.Extras)
	}
	return res
}

func toExternalMessage(msg *model.Message) *model.MessageExternal {
	res := &model.MessageExternal{
		ID:            msg.ID,
		ApplicationID: msg.ApplicationID,
		Message:       msg.Message,
		Title:         msg.Title,
		Priority:      &msg.Priority,
		Date:          msg.Date,
	}
	if len(msg.Extras) != 0 {
		res.Extras = make(map[string]interface{})
		json.Unmarshal(msg.Extras, &res.Extras)
	}
	return res
}

func toExternalMessages(msg []*model.Message) []*model.MessageExternal {
	res := make([]*model.MessageExternal, len(msg))
	for i := range msg {
		res[i] = toExternalMessage(msg[i])
	}
	return res
}
