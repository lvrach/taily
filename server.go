package main

import (
    "crypto/rand"
    "encoding/base64"
    "fmt"
    lz4 "github.com/bkaradzic/go-lz4"
    "github.com/boltdb/bolt"
    "github.com/codahale/blake2"
    "html/template"
    "log"
    "net"
    "net/http"
    "strings"
    "sync"
)

var streams = make(map[string][]string)

type StreamTail map[string][]chan []byte;

var tails StreamTail = make(StreamTail);

func (tails StreamTail) create(id string) {
    tails[id] = make([]chan []byte, 0);
}

func (tails StreamTail) connect(id string, c chan []byte) {
    tails[id] = append(tails[id], c)
    fmt.Printf("connect id: %s connect (%d)\n", id, len(tails[id]))
}

func (tails StreamTail) disconnect(id string, d chan []byte) {
    for i, c := range tails[id] {
        if (c == d) {
            tails[id] = append(tails[id][:i], tails[id][i+1:]...)
        }
    }

    fmt.Printf("disconnect id: %s connect (%d)\n", id, len(tails[id]))

}

func (tails StreamTail) broadcast(id string, data []byte) {
    fmt.Printf("broadcast to id: %d (%d)\n", id, len(tails[id]))
    
    for _, c := range tails[id] {
        c <- data
    }
}

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
    http.HandleFunc("/", httpHandler)
    http.ListenAndServe(":8080", nil)
}

type StreamPage struct {
    Id    string
    Lines []string
}

func httpHandler(w http.ResponseWriter, r *http.Request) {
    t, _ := template.ParseFiles("index.html")

    id := string(r.URL.Path[1:])

    if lines, ok := streams[id]; ok {
        p := &StreamPage{Id: id, Lines: lines}
        t.Execute(w, p)
    } else {
        err := db.View(func(tx *bolt.Tx) error {
            bucket := tx.Bucket([]byte(id))
            if bucket == nil {
                http.NotFound(w, r)
                fmt.Errorf("Bucket %s not found!", id)
                return nil
            }
            var err error

            data := bucket.Get([]byte("body"))
            data, err = lz4.Decode(nil, data)
            if err != nil {
                return err
            }
            fmt.Fprintf(w, string(data))

            return nil
        })

        if err != nil {
            log.Fatal(err)
        }
    }
}

func rawHandler(w http.ResponseWriter, r *http.Request) {
    id := string(r.URL.Path[5:])
    if lines, ok := streams[id]; ok {
        fmt.Fprintf(w, strings.Join(lines, ""))
    } else {
        err := db.View(func(tx *bolt.Tx) error {
            bucket := tx.Bucket([]byte(id))
            if bucket == nil {
                http.NotFound(w, r)
                fmt.Errorf("Bucket %s not found!", id)
                return nil
            }
            var err error

            data := bucket.Get([]byte("body"))
            data, err = lz4.Decode(nil, data)
            if err != nil {
                return err
            }
            fmt.Fprintf(w, string(data))

            return nil
        })

        if err != nil {
            log.Fatal(err)
        }
    }
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


    var tail = make(chan []byte)    

    defer func() {
        tails.disconnect(id, tail)
    }()

    notify := w.(http.CloseNotifier).CloseNotify()
    go func() {
        <-notify
        tails.disconnect(id, tail)
    }()
    
    tails.connect(id, tail)

    for {

        // Write to the ResponseWriter
        // Server Sent Events compatible
        fmt.Fprintf(w, "data: %s\n\n", <-tail)

        // Flush the data immediatly instead of buffering it for later.
        flusher.Flush()
    }
}

func netServer() {
    ln, err := net.Listen("tcp", ":9999")
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

    tails.create(id)
    streams[id] = make([]string, 0)
    conn.Write([]byte(fmt.Sprintf("http://localhost:8080/%s \n", id)))
    for {
        n, err := conn.Read(buf[:])
        if err != nil {
            break
        }
        streams[id] = append(streams[id], string(buf[:n]))
        fmt.Printf("%s", string(buf[:n]))
        tails.broadcast(id, buf[:n])
        
    }
    saveStream(id)
    conn.Close()
}

func saveStream(id string) {
    if lines, ok := streams[id]; ok {
        data := []byte(strings.Join(lines, ""))

        err := db.Update(func(tx *bolt.Tx) error {
            bucket, err := tx.CreateBucketIfNotExists([]byte(id))
            if err != nil {
                return err
            }

            data, err := lz4.Encode(nil, data)
            if err != nil {
                return err
            }

            err = bucket.Put([]byte("body"), data)

            if err != nil {
                return err
            }
            return nil
        })
        if err != nil {
            log.Fatal(err)
        } else {
            delete(streams, id)
        }
    }
}
