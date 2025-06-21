package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/grafana/pyroscope-go"
)

// Эта функция имитирует ресурсоемкую операцию (CPU)
func work(n int) {
	for i := 0; i < n; i++ {
		_ = time.Now().UnixNano()
	}
}

// Эта функция аллоцирует и сохраняет память (куча)
var memleak = make([][]byte, 0)

func memoryHandler(w http.ResponseWriter, r *http.Request) {
	pyroscope.TagWrapper(r.Context(), pyroscope.Labels("handler", "memory"), func(ctx context.Context) {
		log.Println("Handling memory leak request...")
		// Аллоцируем 100 МБ и не отпускаем
		b := make([]byte, 100*1024*1024)
		for i := 0; i < len(b); i += 4096 {
			b[i] = byte(i) // Чтобы реально память выделилась
		}
		memleak = append(memleak, b) // Оставляем ссылку — не GC
		fmt.Fprintln(w, "Memory leak allocated 100MB!")
	})
}

// Эта функция долго читает с диска
func diskHandler(w http.ResponseWriter, r *http.Request) {
	pyroscope.TagWrapper(r.Context(), pyroscope.Labels("handler", "disk"), func(ctx context.Context) {
		log.Println("Handling slow disk request...")
		f, err := ioutil.TempFile("", "pyrotest")
		if err != nil {
			http.Error(w, "Failed to create temp file", 500)
			return
		}
		defer os.Remove(f.Name())
		defer f.Close()
		// Запишем 100 МБ
		data := make([]byte, 1024*1024)
		for i := 0; i < 100; i++ {
			if _, err := f.Write(data); err != nil {
				http.Error(w, "Disk write error", 500)
				return
			}
		}
		_, err = f.Seek(0, 0)
		if err != nil {
			http.Error(w, "Seek error", 500)
			return
		}
		total := 0
		start := time.Now()
		// Медленно читаем файл (делая sleep)
		for {
			n, err := f.Read(data)
			if n > 0 {
				total += n
				time.Sleep(10 * time.Millisecond) // эмулируем медленный диск
			}
			if err != nil {
				break
			}
		}
		fmt.Fprintf(w, "Disk read %d bytes in %v!\n", total, time.Since(start))
	})
}

// Эта функция долго ждет сеть (клиент до публичного ресурса)
func networkHandler(w http.ResponseWriter, r *http.Request) {
	pyroscope.TagWrapper(r.Context(), pyroscope.Labels("handler", "network"), func(ctx context.Context) {
		log.Println("Handling slow network request...")
		start := time.Now()
		conn, err := net.DialTimeout("tcp", "example.org:80", 5*time.Second)
		if err != nil {
			http.Error(w, "Network dial error", 500)
			return
		}
		defer conn.Close()
		req := "GET / HTTP/1.1\r\nHost: example.org\r\nConnection: close\r\n\r\n"
		conn.Write([]byte(req))
		buf := make([]byte, 4096)
		// Читаем с задержкой, эмулируем долгую сеть
		for {
			conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
			_, err := conn.Read(buf)
			if err != nil {
				break
			}
			time.Sleep(200 * time.Millisecond) // замедляем обработку
		}
		fmt.Fprintf(w, "Network call to example.org finished in %v\n", time.Since(start))
	})
}

func slowHandler(w http.ResponseWriter, r *http.Request) {
	pyroscope.TagWrapper(r.Context(), pyroscope.Labels("handler", "slow"), func(ctx context.Context) {
		log.Println("Handling slow request...")
		work(20000000)
		fmt.Fprintln(w, "Slow request handled!")
	})
}

func fastHandler(w http.ResponseWriter, r *http.Request) {
	pyroscope.TagWrapper(r.Context(), pyroscope.Labels("handler", "fast"), func(ctx context.Context) {
		log.Println("Handling fast request...")
		work(5000000)
		fmt.Fprintln(w, "Fast request handled!")
	})
}

func main() {
	hostname, _ := os.Hostname()

	_, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: "my-go-app",
		ServerAddress:   "http://pyroscope-server:4040",
		Logger:          pyroscope.StandardLogger,
		Tags:            map[string]string{"hostname": hostname},
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
		},
	})
	if err != nil {
		log.Fatalf("Failed to start pyroscope: %v", err)
	}

	http.HandleFunc("/slow", slowHandler)
	http.HandleFunc("/fast", fastHandler)
	http.HandleFunc("/memory", memoryHandler)
	http.HandleFunc("/disk", diskHandler)
	http.HandleFunc("/network", networkHandler)

	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
