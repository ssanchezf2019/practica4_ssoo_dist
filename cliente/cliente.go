package main

import (
	"bytes"
	"container/heap"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	buf               bytes.Buffer
	logger            = log.New(&buf, "logger: ", log.Lshortfile)
	msg               string
	estadoActual      int
	mu                sync.Mutex
	colaAviones       AvionHeap                // Cola de prioridad de aviones
	pistasDisponibles = make(chan struct{}, 3) // Canal para manejar pistas disponibles (máximo 3)
	procesar          bool
	detenerProceso    bool
)

// Estructura que representa un avión con sus atributos
type Avion struct {
	id           int
	categoria    string
	numPasajeros int
	prioridad    int // Prioridad en la cola
}

// Implementación de una cola de prioridad para los aviones
type AvionHeap []Avion

func (h AvionHeap) Len() int           { return len(h) }
func (h AvionHeap) Less(i, j int) bool { return h[i].prioridad > h[j].prioridad }
func (h AvionHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

// Agregar un avión a la cola
func (h *AvionHeap) Push(x interface{}) {
	*h = append(*h, x.(Avion))
}

// Retirar un avión de la cola
func (h *AvionHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// Inicialización del programa
func init() {
	rand.Seed(time.Now().UnixNano()) // Semilla para números aleatorios
	for i := 0; i < 3; i++ {
		pistasDisponibles <- struct{}{} // Inicializa 3 pistas disponibles
	}
	heap.Init(&colaAviones) // Inicializa la cola de prioridad
	// Agrega aviones de prueba con diferentes categorías y prioridades
	for i := 1; i <= 10; i++ {
		heap.Push(&colaAviones, Avion{id: i, categoria: "A", numPasajeros: rand.Intn(50) + 101, prioridad: 0})
		heap.Push(&colaAviones, Avion{id: i + 10, categoria: "B", numPasajeros: rand.Intn(51) + 50, prioridad: 0})
		heap.Push(&colaAviones, Avion{id: i + 20, categoria: "C", numPasajeros: rand.Intn(50) + 1, prioridad: 0})
	}
	procesar = false
	detenerProceso = false
}

// Punto de entrada principal
func main() {
	for {
		// Intenta conectar con el servidor
		conn, err := net.Dial("tcp", "localhost:8000")
		if err != nil {
			logger.Println("Error al conectar con el servidor:", err)
			time.Sleep(2 * time.Second)
			continue
		}
		defer conn.Close()

		// Procesa la cola en segundo plano
		go procesarCola()
		// Lee los mensajes del servidor
		leerMensajes(conn)
	}
}

// Leer mensajes del servidor y manejar el estado del aeropuerto
func leerMensajes(conn net.Conn) {
	buf := make([]byte, 512)
	for {
		n, err := conn.Read(buf)
		if err == io.EOF {
			fmt.Println("Conexión cerrada por el servidor")
			return
		}
		if err != nil {
			fmt.Println("Error al leer del servidor:", err)
			return
		}
		if n > 0 {
			msg = strings.TrimSpace(string(buf[:n]))
			// Intenta interpretar el mensaje como un estado
			if estado, err := strconv.Atoi(msg); err == nil {
				actualizarEstado(estado)
				descripcionEstado(estado)
			} else {
				fmt.Println(msg)
			}
		}
	}
}

// Imprime la descripción del estado actual
func descripcionEstado(estado int) {
	descripciones := map[int]string{
		0: "Aeropuerto inactivo",
		1: "Solo categoría A",
		2: "Solo categoría B",
		3: "Solo categoría C",
		4: "Prioridad categoría A",
		5: "Prioridad categoría B",
		6: "Prioridad categoría C",
		9: "Aeropuerto cerrado temporalmente",
	}
	if desc, ok := descripciones[estado]; ok {
		fmt.Printf("(%s)\n", desc)
	}
}

// Actualiza el estado del aeropuerto y reorganiza la cola si es necesario
func actualizarEstado(estado int) {
	mu.Lock()
	defer mu.Unlock()

	if estado == 7 || estado == 8 {
		fmt.Println("Estado 7 u 8 recibido, se mantiene el estado actual:", estadoActual)
		return
	}

	estadoActual = estado
	procesar = estado >= 1 && estado <= 6 // Habilitar procesamiento según el estado
	detenerProceso = false
	fmt.Printf("Estado actualizado a: %d ", estadoActual)
	if procesar {
		reordenarCola()
	}
}

// Reorganiza la cola de prioridad de acuerdo al estado actual
func reordenarCola() {
	var nuevaCola AvionHeap
	for colaAviones.Len() > 0 {
		avion := heap.Pop(&colaAviones).(Avion)
		avion.prioridad = calcularPrioridad(avion) // Recalcula la prioridad
		heap.Push(&nuevaCola, avion)
	}
	colaAviones = nuevaCola
}

// Calcula la prioridad de un avión basado en el estado actual
func calcularPrioridad(avion Avion) int {
	switch estadoActual {
	case 1:
		if avion.categoria == "A" {
			return 1
		}
	case 2:
		if avion.categoria == "B" {
			return 1
		}
	case 3:
		if avion.categoria == "C" {
			return 1
		}
	case 4:
		if avion.categoria == "A" {
			return 2
		}
	case 5:
		if avion.categoria == "B" {
			return 2
		}
	case 6:
		if avion.categoria == "C" {
			return 2
		}
	}
	return 0
}

// Procesa la cola de aviones y asigna pistas disponibles
func procesarCola() {
	for {
		mu.Lock()
		if detenerProceso {
			mu.Unlock()
			time.Sleep(1 * time.Second)
			continue
		}
		if !procesar || colaAviones.Len() == 0 {
			mu.Unlock()
			time.Sleep(1 * time.Second)
			continue
		}

		avion := heap.Pop(&colaAviones).(Avion)
		if estadoActual >= 1 && estadoActual <= 3 && !esCategoriaValida(avion) {
			fmt.Printf("Todos los aviones de la categoría %s han sido procesados.\n", categoriaEstado(estadoActual))
			heap.Push(&colaAviones, avion) // Reinsertar el avión en la cola
			detenerProceso = true
			mu.Unlock()
			continue
		}
		mu.Unlock()
		usarPista(avion)
	}
}

// Devuelve la categoría correspondiente al estado
func categoriaEstado(estado int) string {
	switch estado {
	case 1:
		return "A"
	case 2:
		return "B"
	case 3:
		return "C"
	}
	return ""
}

// Verifica si un avión pertenece a la categoría válida para el estado actual
func esCategoriaValida(avion Avion) bool {
	switch estadoActual {
	case 1:
		return avion.categoria == "A"
	case 2:
		return avion.categoria == "B"
	case 3:
		return avion.categoria == "C"
	}
	return true
}

// Simula el uso de una pista por un avión
func usarPista(avion Avion) {
	<-pistasDisponibles // Reserva una pista
	fmt.Printf("Avión %d (%s) está usando una pista\n", avion.id, avion.categoria)
	time.Sleep(time.Duration(rand.Intn(4)) * time.Second) // Simula tiempo de uso
	fmt.Printf("Avión %d (%s) ha terminado de usar la pista\n", avion.id, avion.categoria)
	pistasDisponibles <- struct{}{} // Libera la pista
}
