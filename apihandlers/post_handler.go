package apihandlers

import (
	"encoding/json"
	"net/http"

	"git.01.alem.school/qjawko/forum/dto"
	"git.01.alem.school/qjawko/forum/http_errors"
	"git.01.alem.school/qjawko/forum/model"
	"git.01.alem.school/qjawko/forum/service"
	"git.01.alem.school/qjawko/forum/utils"
	uuid "github.com/satori/go.uuid"
)

type PostHandler struct {
	SubforumService     *service.SubforumService
	SubforumRoleService *service.SubforumRoleService
	PostService         *service.PostService
	UserService         *service.UserService
	likeService         *service.LikeService
	CommentService      *service.CommentService
	Endpoint            string
}

func NewPostHandler(endpoint string,
	likeService *service.LikeService,
	postService *service.PostService,
	commentService *service.CommentService,
	userService *service.UserService,
	subforumService *service.SubforumService,
	roleService *service.SubforumRoleService) *PostHandler {

	return &PostHandler{
		likeService:         likeService,
		SubforumRoleService: roleService,
		SubforumService:     subforumService,
		PostService:         postService,
		CommentService:      commentService,
		UserService:         userService,
		Endpoint:            endpoint,
	}
}

func contains(roles []model.SubforumRole, id uuid.UUID) bool {
	for _, r := range roles {
		if r.ID == id {
			return true
		}
	}

	return false
}

func (ph *PostHandler) checkForPostAuthority(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := utils.GetUser(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		id := r.URL.Path[len(ph.Endpoint):]
		post, err := ph.PostService.GetPostById(uuid.FromStringOrNil(id))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		subforum, err := ph.SubforumService.GetSubforumById(post.SubforumID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		admins, err := ph.SubforumRoleService.GetBySubforumId(subforum.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		if user.ID == post.UserID || contains(admins, user.ID) {
			next.ServeHTTP(w, r)
		} else {
			http.Error(w, "У вас нет прав", http.StatusBadRequest)
			return
		}
	})
}

func (ph *PostHandler) Route(w http.ResponseWriter, r *http.Request) {
	var funcToCall func(http.ResponseWriter, *http.Request)

	switch r.Method {
	case http.MethodGet:
		endpoint := r.URL.Path[len(ph.Endpoint):]
		if len(endpoint) == 0 {
			funcToCall = ph.GetAll
		} else {
			funcToCall = ph.GetPostByID
		}

	case http.MethodPost:
		funcToCall = ph.CreatePost
	case http.MethodPut:
		funcToCall = ph.checkForPostAuthority(http.HandlerFunc(ph.Update)).ServeHTTP
	case http.MethodDelete:
		funcToCall = ph.checkForPostAuthority(http.HandlerFunc(ph.Delete)).ServeHTTP
	default:
		http.Error(w, "Route Not found", http.StatusNotFound)
		return
	}

	funcToCall(w, r)
}

//CreatePost q
func (ph *PostHandler) CreatePost(w http.ResponseWriter, r *http.Request) {
	var post model.Post
	if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created, err := ph.PostService.CreatePost(&post)
	if err != nil {
		httpErr := err.(*http_errors.HttpError)
		http.Error(w, httpErr.Error(), httpErr.Code)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

//GetAll qwe
func (ph *PostHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	posts, err := ph.PostService.GetAllPosts()
	if err != nil {
		httpErr := err.(*http_errors.HttpError)
		http.Error(w, httpErr.Error(), httpErr.Code)
		return
	}

	userID := r.FormValue("userid")
	subforumID := r.FormValue("subforumid")

	if userID != "" {
		posts = utils.PostsFilter(posts, func(post model.Post) bool {
			return post.UserID == uuid.FromStringOrNil(userID)
		})
	}

	if subforumID != "" {
		posts = utils.PostsFilter(posts, func(post model.Post) bool {
			return post.SubforumID == uuid.FromStringOrNil(subforumID)
		})
	}

	json.NewEncoder(w).Encode(posts)
}

//GetPostByID возвращает postDto, который хранит пост с указанным ID и его комментарии
func (ph *PostHandler) GetPostByID(w http.ResponseWriter, r *http.Request) {

	id := r.URL.Path[len(ph.Endpoint):]

	post, err := ph.PostService.GetPostById(uuid.FromStringOrNil(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	comments, err := ph.CommentService.GetAllCommentsByPostID(uuid.FromStringOrNil(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	subforum, err := ph.SubforumService.GetSubforumById(post.SubforumID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	user, err := ph.UserService.GetUserByID(post.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rates, err := ph.likeService.GetLikesByPostID(post.ID)

	postDto := &dto.PostDto{
		ID:           post.ID,
		Title:        post.Title,
		Content:      post.Content,
		CreationDate: post.CreationDate,
		Subforum:     subforum,
		User:         user,
		Comments:     comments,
		Likes:        rates,
	}

	json.NewEncoder(w).Encode(postDto)
}

func (ph *PostHandler) Update(w http.ResponseWriter, r *http.Request) {
	var post model.Post
	if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
		http.Error(w, "Bad Body", http.StatusBadRequest)
		return
	}

	idFromURL := r.URL.Path[len(ph.Endpoint):]
	post.ID = uuid.FromStringOrNil(idFromURL)

	updated, err := ph.PostService.UpdatePost(&post)
	if err != nil {
		httpErr := err.(*http_errors.HttpError)
		http.Error(w, httpErr.Error(), httpErr.Code)
		return
	}

	json.NewEncoder(w).Encode(updated)
}

func (ph *PostHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len(ph.Endpoint):]
	if err := ph.PostService.DeletePost(uuid.FromStringOrNil(id)); err != nil {
		httpErr := err.(*http_errors.HttpError)
		http.Error(w, httpErr.Error(), httpErr.Code)
		return
	}
}
