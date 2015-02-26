package main

import (
    "fmt"
     lz4 "github.com/bkaradzic/go-lz4"
     "github.com/boltdb/bolt"
    // "log"
     "strings"
)

type TailStream chan []byte;

type LongStream struct {
	id string
	Lenght int
	Lines []string
	Tails []TailStream
}

var streams = make(map[string]*LongStream)

func NewStream(id string) *LongStream {
	stream := new(LongStream)
    stream.id = id
	streams[id] = stream
	return stream
}

func GetStream(id string) *LongStream {
	if stream, ok := streams[id]; ok {
		return stream;
	}

    stream := NewStream(id)
    
    err := db.View(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte(id))
        
        if bucket == nil {
            fmt.Errorf("Bucket %s not found!", id)
            return nil
        }
        
        var err error

        data := bucket.Get([]byte("body"))
        data, err = lz4.Decode(nil, data)
        if err != nil {
           return err
        }
        stream.Lines = strings.Split(string(data), "\n")
        return nil
    })
    if (err != nil) {
        panic(err)
    }        
    return stream;
   
}

func (stream *LongStream) Push(line string) {
	stream.Lines = append(stream.Lines, line);
    for _, c := range stream.Tails {
        c <- []byte(line)
    }
}

func (stream *LongStream) Save() {

    data := []byte(strings.Join(stream.Lines, "\n"))
    //TODO erro handling
    data, _ = lz4.Encode(nil, data)


    _ = db.Update(func(tx *bolt.Tx) error {
        bucket, err := tx.CreateBucketIfNotExists([]byte(stream.id))
        if err != nil {
            return err
        }

        err = bucket.Put([]byte("body"), data)

        if err != nil {
            return err
        }
        return nil
    })
}

func (stream *LongStream) Follow() (chan []byte){
	channel := make(chan []byte);
    stream.Tails = append(stream.Tails, channel)
    fmt.Printf("huid: %s connect (%d)\n", stream.id, len(stream.Tails))
    return channel
}

func (stream *LongStream) Unfollow(d chan []byte) {
    for i, c := range stream.Tails {
        if (c == d) {
            stream.Tails = append(stream.Tails[:i], stream.Tails[i+1:]...)
        }
    }
    fmt.Printf("huid: %s disconnect \n", stream.id)
}