package main

import (
    "crypto/rand"
    "encoding/base64"
    "fmt"
//    lz4 "github.com/bkaradzic/go-lz4"
    "github.com/boltdb/bolt"
    "github.com/codahale/blake2"
    "html/template"
    "log"
    "net"
    "net/http"
    "strings"
    "sync"
)


var db *bolt.DB

func main() {
    var wg sync.WaitGroup
    var err error
    db, err = bolt.Open("./bolt.db", 0644, nil)
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    wg.Add(2)

    go httpServer()
    go netServer()

    wg.Wait()
}

var count int = 0

var base64url = base64.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_")

func huuid() string {

    hash := blake2.New(&blake2.Config{Size: 33})

    bytes := make([]byte, 256)
    rand.Read(bytes)
    hash.Write(bytes)

    b64 := base64url.EncodeToString([]byte(hash.Sum(nil)))
    return strings.Replace(b64, "/", "_", -1)
}

func httpServer() {
    http.HandleFunc("/tail/", sseHandler)
    http.HandleFunc("/raw/", rawHandler)
    http.HandleFunc("/view/", viewerHandler)
    http.Handle("/", http.FileServer(http.Dir("./static")))
    http.ListenAndServe(":8080", nil)
}

type StreamPage struct {
    Id    string
    Lines []string
}

func viewerHandler(w http.ResponseWriter, r *http.Request) {
    t, _ := template.ParseFiles("views/viewer.html")
    id := string(r.URL.Path[len("/view/"):])

    stream := GetStream(id)

    p := &StreamPage{Id: id, Lines: stream.Lines}
    t.Execute(w, p)
}

func rawHandler(w http.ResponseWriter, r *http.Request) {
    id := string(r.URL.Path[5:])
    stream := GetStream(id)
    fmt.Fprintf(w, strings.Join(stream.Lines, "\n"))
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
    id := string(r.URL.Path[6:])

    flusher, ok := w.(http.Flusher)

    if !ok {
        http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("Access-Control-Allow-Origin", "*")


    stream := GetStream(id)
    tail := stream.Follow()

    defer func() {
        stream.Unfollow(tail)
    }()

    notify := w.(http.CloseNotifier).CloseNotify()
    go func() {
        <-notify
        stream.Unfollow(tail)
    }()

    for {
        // Write to the ResponseWriter
        // Server Sent Events compatible
        fmt.Fprintf(w, "data: %s\n\n", <-tail)

        // Flush the data immediatly instead of buffering it for later.
        flusher.Flush()
    }
}

func netServer() {
    ln, err := net.Listen("tcp", ":4444")
    if err != nil {
        panic(err)
    }
    for {
        conn, err := ln.Accept()
        if err != nil {
            // handle error
        }
        go handleConnection(conn)
    }
}

func handleConnection(conn net.Conn) {
    var buf [1024]byte
    id := huuid()

    stream := NewStream(id)
    conn.Write([]byte(fmt.Sprintf("http://localhost:8080/view/%s \n", id)))
    rest := "";
    for {
        n, err := conn.Read(buf[:])
        if err != nil {
            break
        }
        lines := strings.Split(fmt.Sprint(rest, string(buf[:n])), "\n")
        for _, line := range lines[:len(lines)-1] {
            stream.Push(line);
        }
        rest = lines[len(lines)-1];
    }
    stream.Push(rest);
    stream.Save()
    conn.Close()
}