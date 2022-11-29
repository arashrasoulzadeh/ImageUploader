package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
	uuid "github.com/google/uuid"
	"github.com/h2non/bimg"
	"github.com/jedib0t/go-pretty/table"
)

var endpoint string
var key string
var secret string

type Image struct {
	Source string `uri:"source" binding:"required"`
	Path   string `uri:"path" binding:"required"`
}

func main() {

	endpoint = "sfo3.digitaloceanspaces.com"
	key = ""
	secret = os.Getenv("S3_SECRET")

	router := gin.Default()
	router.GET("/:source/:path", func(c *gin.Context) {
		params := c.Request.URL.Query()
		ext := "jpeg"
		var image Image
		if err := c.ShouldBindUri(&image); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()})
			return
		}
		imagePath := "/tmp/fm/" + image.Path

		byteFile, err := ioutil.ReadFile(imagePath)
		if err != nil {
			fmt.Println(err)
		}
		img := bimg.NewImage(byteFile)

		converted := img.Image()
		if _, con := params["c"]; con {
			f := c.Query("c")
			if f == "png" {
				converted, _ = img.Convert(bimg.PNG)
			}
			if f == "gif" {
				converted, _ = img.Convert(bimg.GIF)
			}
			if f == "webp" {
				converted, _ = img.Convert(bimg.WEBP)
			}
		}

		ext = img.Type()

		if _, ws := params["s"]; ws {
			fmt.Println("Should resize ", params)
			size, err := img.Size()
			s := c.Query("s")
			w := size.Width
			h := size.Height

			if s == "s" {
				w, h = createSize(w, h, 320)
			}
			if s == "m" {
				w, h = createSize(w, h, 640)
			}
			if s == "l" {
				w, h = createSize(w, h, 860)
			}
			if s == "o" {

			}

			processed, err := bimg.NewImage(converted).Process(bimg.Options{Quality: 100, Width: w, Height: h})
			if err == nil {
				c.Header("Content-type", "image/"+ext)
				c.Header("Cache-Control", "max-age=3600")
				c.Data(http.StatusOK, "application/octet-stream", processed)
			}
		}

		processed, err := bimg.NewImage(converted).Process(bimg.Options{})
		// c.Header("Content-Disposition", "attachment; filename="+image.Path)
		c.Header("Content-type", "image/"+ext)

		c.Data(http.StatusOK, "application/octet-stream", processed)

		// c.JSON(http.StatusOK, gin.H{"name": image.Source, "uuid": imagePath})
	})

	router.MaxMultipartMemory = 8 << 20 // 8 MiB
	router.POST("/upload", func(c *gin.Context) {
		// Single file
		file, _ := c.FormFile("file")
		// Upload the file to specific dst.
		hash := uuid.NewString()
		dir := "/tmp/fm/"
		filename := hash + "_" + file.Filename
		dest := dir + filename
		c.SaveUploadedFile(file, dest)
		newPath := CreateConvertedImage(dest, dir)
		go UploadToS3("revi", dest)

		c.JSON(http.StatusOK, gin.H{
			"code":         http.StatusOK,
			"original_src": dest,
			"src":          newPath,
			"path":         endpoint + "/" + filename,
		})

	})
	router.Run(":8080")

	router.Run()
}

func CreateConvertedImage(path string, dir string) (newPath string) {
	file, err_open := os.Open(path)
	if err_open != nil {
		fmt.Println("failed to open file " + path)
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	var size int64 = fileInfo.Size()
	buffer := make([]byte, size)
	file.Read(buffer)
	newPath, _ = ConvertImage(buffer, 100, dir)
	return newPath
}

func UploadToS3(bucket string, path string) {

	// All clients require a Session. The Session provides the client with
	// shared configuration such as region, endpoint, and credentials. A
	// Session should be shared where possible to take advantage of
	// configuration and credential caching. See the session package for
	// more information.
	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(key, secret, ""),
		Endpoint:         aws.String(endpoint),
		Region:           aws.String("sfo3"),
		S3ForcePathStyle: aws.Bool(false), // Depending on your version, alternatively use o.UsePathStyle = false
	}
	sess := session.New(s3Config)

	// Create a new instance of the service's client with a Session.
	// Optional aws.Config values can also be provided as variadic arguments
	// to the New function. This option allows you to provide service
	// specific configuration.
	svc := s3.New(sess)

	// Create a context with a timeout that will abort the upload if it takes
	// more than the passed in timeout.
	ctx := context.Background()
	var cancelFn func()
	// Ensure the context is canceled to prevent leaking.
	// See context package for more information, https://golang.org/pkg/context/
	if cancelFn != nil {
		defer cancelFn()
	}

	file, err_open := os.Open(path)
	if err_open != nil {
		fmt.Println("failed to open file " + path)
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	var size int64 = fileInfo.Size()
	buffer := make([]byte, size)
	file.Read(buffer)

	// Uploads the object to S3. The Context will interrupt the request if the
	// timeout expires.
	_, err := svc.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("test/a.jpg"),
		Body:   bytes.NewReader(buffer),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == request.CanceledErrorCode {
			// If the SDK can determine the request or retry delay was canceled
			// by a context the CanceledErrorCode error code will be returned.
			fmt.Fprintf(os.Stderr, "upload canceled due to timeout, %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "failed to upload object, %v\n", err)
		}
	}

	fmt.Println(fmt.Sprintf("successfully uploaded file to %s/%s\n", bucket, key))

}

// The mime type of the image is changed, it is compressed and then saved in the specified folder.
func ConvertImage(buffer []byte, quality int, dirname string) (string, error) {
	filename := strings.Replace(uuid.New().String(), "-", "", -1)
	img := bimg.NewImage(buffer)
	converted, err := img.Convert(bimg.JPEG)
	if err != nil {
		return filename, err
	}

	size, err := img.Size()

	width := size.Width
	height := size.Height
	pixels := width * height
	maxWidth := 1920
	maxHeight := 1080
	maxPixels := 2073600
	originalPixels := width * height
	status := "OK!"
	newWidth := 0
	newHeight := 0
	newPixels := newWidth * newHeight

	if pixels > maxPixels {
		status = "DOWN SCALE"
		newWidth, newHeight = DownScale(width, height, maxWidth, maxHeight)
	}

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"File Name", "Width", "Height", "Pixels", "Status"})
	t.AppendRow([]interface{}{filename, width, height, originalPixels, status})

	// start converting
	processed, err := bimg.NewImage(converted).Process(bimg.Options{Quality: quality, Width: newWidth, Height: newHeight})

	if err != nil {
		return filename, err
	}
	newPixels = newWidth * newHeight
	newPath := fmt.Sprintf(dirname+"%s.jpg", filename)
	writeError := bimg.Write(newPath, processed)
	rate := 100 - ((float64(newPixels) / float64(originalPixels)) * 100)

	if writeError != nil {
		fmt.Println("did not convert in "+newPath, writeError)
		status = "FAILED!"
		t.AppendRow([]interface{}{filename, newWidth, newHeight, strconv.Itoa(newPixels) + " (" + fmt.Sprintf("%.2f", rate) + "%)", status})
		t.Render()
		return filename, writeError
	}
	// fmt.Println("converted in " + newPath)
	status = "OK!"
	t.AppendRow([]interface{}{filename, newWidth, newHeight, strconv.Itoa(newPixels) + " (" + fmt.Sprintf("%.2f", rate) + "%)", status})
	t.Render()

	return newPath, nil
}

func DownScale(w int, h int, mw int, mh int) (nw int, nh int) {
	ratio := 0.0

	if h >= mh {
		ratio = float64(mh) / float64(h)
	}

	if w >= mw {
		ratio = float64(mw) / float64(w)
	}
	return int(float64(w) * ratio), int(float64(h) * ratio)
}

func createSize(iw int, ih int, max int) (w int, h int) {
	if iw > max {
		ratio := iw / max
		w = iw / ratio
		h = ih / ratio
	}
	return w, h
}
