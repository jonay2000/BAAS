package api

import (
	"encoding/json"
	"fmt"
	"github.com/baas-project/baas/pkg/fs"
	"github.com/baas-project/baas/pkg/images"
	"github.com/baas-project/baas/pkg/model"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"strconv"
)

// CreateImage creates an image based on a name
// Example request: POST user/Jan/image
// Example body: {"DiskUUID": "30DF-844C", "Name": "Fedora"}
// Example response: {"Name": "Fedora",
//                    "Versions": [{"Version": "2021-11-01T00:11:22.38125222+01:00",
//                                  "ImageModelID": 0}],
//                    "UUID": "eed13670-5974-4c98-b044-347e1f630bc5",
//                    "DiskUUID": "30DF-844C",
//                    "UserModelID": 0}
func (api_ *API) CreateImage(w http.ResponseWriter, r *http.Request) {
	name, err := GetName(w, r)
	if err != nil {
		return
	}

	image := images.ImageModel{}
	err = json.NewDecoder(r.Body).Decode(&image)

	// Input validation
	if image.Name == "" {
		http.Error(w, "Name is not allowed to be empty", http.StatusBadRequest)
		return
	}

	if len(image.Versions) != 0 {
		http.Error(w, "There shouldn't be a version", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "couldn't decode image model", http.StatusBadRequest)
		log.Errorf("decode image model: %v", err)
		return
	}

	// Generate the UUID and create the entry in the database.
	// We don't actually make an image file yet.
	image.UUID = images.ImageUUID(uuid.New().String())
	image.Username = name
	api_.store.CreateImage(&image)

	if err != nil {
		http.Error(w, "couldn't create image model", http.StatusInternalServerError)
		log.Errorf("decode create model: %v", err)
		return
	}

	// Create the actual image together with the first empty version which a user may or may not use.
	err = os.Mkdir(fmt.Sprintf(api_.diskpath+"/%s", image.UUID), os.ModePerm)
	if err != nil {
		http.Error(w, "could not create image", http.StatusInternalServerError)
		log.Errorf("cannot create image directory: %v", err)
		return
	}

	err = image.CreateImageFile(imageFileSize, api_.diskpath, images.SizeMegabyte)

	if err != nil {
		http.Error(w, "Cannot create the image file", http.StatusInternalServerError)
		log.Errorf("image creation failed: %v", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(&image)
}

// GetImagesByUser fetches all the images of the given user
// Example request: user/Jan/images
// Example result: [
//  {
//    "Name": "Windows",
//    "Versions "a9c11954-6161-410b-b238-c03df5c529e9",
//    "DiskUUID": "30DF-844C",
//    "UserModelID": 2
//  },
//  {
//    "Name": "Arch Linux",
//    "Versions": null,
//    "UUID": "341b2c69-8776-4e54-9330-7c9692f7ed28",
//    "DiskUUID": "30DF-844C",
//    "UserModelID": 2
//  }
//]
func (api_ *API) GetImagesByUser(w http.ResponseWriter, r *http.Request) {
	name, err := GetName(w, r)
	if err != nil {
		return
	}

	userImages, err := api_.store.GetImagesByUsername(name)

	if err != nil {
		http.Error(w, "couldn't get userImages", http.StatusInternalServerError)
		log.Errorf("get userImages by users: %v", err)
		return
	}

	_ = json.NewEncoder(w).Encode(userImages)
}

// GetImagesByName gets any image based on the user who created it and human-readable name assigned to it.
// Example Request: user/Jan/images/Gentoo
// Example Response: [
//  {
//    "Name": "Gentoo",
//    "Versions": null,
//    "UUID": "57bf0cd3-c2bf-4257-acdd-b7f1c8633fcf",
//    "DiskUUID": "30DF-844C",
//    "UserModelID": 1
//  }
//]
func (api_ *API) GetImagesByName(w http.ResponseWriter, r *http.Request) {
	username, err := GetName(w, r)
	if err != nil {
		return
	}

	imageName, err := GetTag("image_name", w, r)
	if err != nil {
		return
	}

	userImages, err := api_.store.GetImagesByNameAndUsername(imageName, username)

	if err != nil {
		http.Error(w, "couldn't get image", http.StatusInternalServerError)
		log.Errorf("get image by name: %v", err)
		return
	}

	_ = json.NewEncoder(w).Encode(userImages)
}

// GetImage gets any image based on it's unique id.
// Example request: image/57bf0cd3-c2bf-4257-acdd-b7f1c8633fcf
// Example response: {
//  "Name": "Gentoo",
//  "Versions": null,
//  "UUID": "57bf0cd3-c2bf-4257-acdd-b7f1c8633fcf",
//  "DiskUUID": "30DF-844C",
//  "UserModelID": 1
//}
func (api_ *API) GetImage(w http.ResponseWriter, r *http.Request) {
	uniqueID, err := GetTag("uuid", w, r)
	if err != nil {
		return
	}

	res, err := api_.store.GetImageByUUID(images.ImageUUID(uniqueID))
	if err != nil {
		http.Error(w, "couldn't get image", http.StatusInternalServerError)
		log.Errorf("get image: %v", err)
		return
	}

	_ = json.NewEncoder(w).Encode(res)
}

// DownloadImageFile gets the specified version of the image off the disk and offers it to the client
func DownloadImageFile(uniqueID string, version string, api *API, w http.ResponseWriter) {
	f, err := os.Open(fmt.Sprintf(api.diskpath+images.FilePathFmt, uniqueID, version))
	if err != nil {
		http.Error(w, "Cannot download the image", http.StatusNotFound)
		log.Errorf("Download image: %v", err)
		return
	}

	// Defer closing the file until the end of the program
	defer func() {
		err = f.Close()
		if err != nil {
			http.Error(w, "Cannot close image file", http.StatusInternalServerError)
			log.Errorf("Cannot close image file: %v", err)
		}
	}()

	// We set the Content-Type to disk/raw as a placeholder, but it does not actually exist. It might be nice to change
	// this at some later date some more common value.
	w.Header().Set("Content-Type", "disk/raw")
	_, err = io.Copy(w, f)

	if err != nil {
		http.Error(w, "Cannot serve image", http.StatusInternalServerError)
		log.Errorf("Cannot serve image: %v", err)
		return
	}
}

// DownloadImage offers the requested image to the respective client
// Example request: image/87f58936-9540-4dad-aba6-253f06142166/1635888918
func (api_ *API) DownloadImage(w http.ResponseWriter, r *http.Request) {
	version, err := GetTag("version", w, r)
	if err != nil {
		http.Error(w, "Invalid version in the URI", http.StatusInternalServerError)
		log.Errorf("Download image: %v", err)
		return
	}

	uniqueID, err := GetTag("uuid", w, r)
	if err != nil {
		http.Error(w, "Invalid uuid in the URI", http.StatusInternalServerError)
		log.Errorf("Download image: %v", err)
		return
	}

	DownloadImageFile(uniqueID, version, api_, w)
}

// DownloadLatestImage offers the latest version
// Example request: image/87f58936-9540-4dad-aba6-253f06142166/latest
func (api_ *API) DownloadLatestImage(w http.ResponseWriter, r *http.Request) {
	uniqueID, err := GetTag("uuid", w, r)
	if err != nil {
		http.Error(w, "Invalid uuid in the URI", http.StatusInternalServerError)
		log.Errorf("Download image: %v", err)
		return
	}

	image, err := api_.store.GetImageByUUID(images.ImageUUID(uniqueID))
	if err != nil {
		http.Error(w, "Invalid uuid in the URI", http.StatusInternalServerError)
		log.Errorf("Download latest image: %v", err)
		return
	}

	version := image.Versions[len(image.Versions)-1]

	DownloadImageFile(uniqueID, strconv.FormatUint(version.Version, 10), api_, w)
}

// UploadImage takes the uploaded file and stores as a new version of the image
// Example request: image/87f58936-9540-4dad-aba6-253f06142166 -H "Content-Type: multipart/form-data"
//                     -F "file=@/tmp/test3.img"
// Example return: Successfully uploaded image: 134251234
func (api_ *API) UploadImage(w http.ResponseWriter, r *http.Request) {
	log.Info("Started with upload")
	uniqueID, err := GetTag("uuid", w, r)
	if err != nil {
		return
	}

	// Get the reader to the multireader
	mr, err := r.MultipartReader()

	if err != nil {
		http.Error(w, "Cannot parse POST form", http.StatusBadRequest)
		log.Errorf("Cannot parse POST form: %v", err)
		return
	}

	// We only use the first part right now, but this might change
	p, err := mr.NextPart()

	// Go error's handling is pain
	if err != nil {
		http.Error(w, "File upload failed", http.StatusInternalServerError)
		log.Errorf("Cannot upload image: %v", err)
		return
	}

	// One liner which closes the file at the end of the call.
	defer func() {
		if err = p.Close(); err != nil {
			log.Errorf("Cannot close upload file: %v", err)
		}
	}()

	version, err := CreateNewVersion(uniqueID, api_.store)
	if err != nil {
		http.Error(w, "cannot fetch the image from the database", http.StatusNotFound)
		log.Errorf("cannot fetch image from database: %v", err)
		return
	}

	// Write the file onto the disk
	dest, err := os.OpenFile(fmt.Sprintf(api_.diskpath+images.FilePathFmt, uniqueID, version.Version), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		http.Error(w, "Cannot open destination file", http.StatusInternalServerError)
		log.Errorf("Cannot open destination file: %v", err)
		return
	}

	err = fs.CopyStream(p, dest)
	if err != nil {
		http.Error(w, "Cannot copy over the contents of the file", http.StatusInternalServerError)
		log.Errorf("Cannot copy the contents of the file: %v", err)
		return
	}

	defer func() {
		if err := dest.Close(); err != nil {
			log.Errorf("Cannot close upload file: %v", err)
		}
	}()
	http.Error(w, "Successfully uploaded image: "+strconv.FormatUint(version.Version, 10), http.StatusOK)
}

// RegisterImageHandlers sets the metadata for each of the routes and registers them to the global handler
func (api_ *API) RegisterImageHandlers() {
	api_.Routes = append(api_.Routes, Route{
		URI:         "/image/{uuid}",
		Permissions: []model.UserRole{model.User, model.Moderator, model.Admin},
		UserAllowed: true,
		Handler:     api_.GetImage,
		Method:      http.MethodGet,
		Description: "Gets information about an image",
	})

	api_.Routes = append(api_.Routes, Route{
		URI:         "/image/{uuid}/latest",
		Permissions: []model.UserRole{model.Moderator, model.Admin},
		UserAllowed: true,
		Handler:     api_.DownloadLatestImage,
		Method:      http.MethodPost,
		Description: "Offers the latest version of the image",
	})

	api_.Routes = append(api_.Routes, Route{
		URI:         "/image/{uuid}/{version}",
		Permissions: []model.UserRole{model.Moderator, model.Admin},
		UserAllowed: true,
		Handler:     api_.DownloadImage,
		Method:      http.MethodGet,
		Description: "Requests a particular version of the image",
	})

	api_.Routes = append(api_.Routes, Route{
		URI:         "/image/{uuid}",
		Permissions: []model.UserRole{model.User, model.Moderator, model.Admin},
		UserAllowed: true,
		Handler:     api_.UploadImage,
		Method:      http.MethodPost,
		Description: "Uploads a new version of the image",
	})
}
