package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/zlepper/encoding-html"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

// Resumen de una oferta.Contiene el URI y el titulo de la misma.
type offertSummary struct {
	Title string `css:".listing a"`
	Link  string `css:".listing a" extract:"attr" attr:"href"`
}

// Recividor para chequear las actualizaciones del bot.
type Reciver struct {
	OK          bool     `json:"ok"`
	Result      []Update `json:"result"`
	Description string   `json:"description"`
}

//Oferta.
type Offert struct {
	Title        string       `css:".ad-details h2"`
	Date         string       `css:".ad-details p"`
	Description  string       `css:".ad-details .ad-description"`
	PanelContact PanelContact `css:".ad-details .panel-body"`
}

//Panel donde se muestra los contactos de la oferta.
type PanelContact struct {
	Lead    string   `css:".lead"`
	Contact []string `css:"strong"`
}

//Lista de los resumenes de ofertas.
type OffertSummarySlice struct {
	OffertSummaries []offertSummary `css:".listings .listing"`
}

// Token de acceso del bot.
var Token string

// Error si el link existe.
var ErrLinkExist = errors.New("Offert exist")

// Error si la pagina ha sido leida.
var ErrPagRead = errors.New("Page readed")

// IDs de los chat que tienen abierta una conversacion con el bot.
var ChatIds []string

var wg = sync.WaitGroup{}

func main() {

	Token = os.Getenv("TOKEN_BOT")
	if Token == "" {
		logrus.Fatal("The TOKEN_BOT EnvVar is empty.")
	}
	var err error
	logrus.Info("Geting chat_ids.")
	ChatIds, err = getChatIds()
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.Info("Main Start.")

	wg.Add(len(ChatIds))

	//Administrar un hilo de la aplicacion por cada id.
	for _, id := range ChatIds {
		go appManager(id)
	}

	//Busca nuevos ids.
	go checkForNewIds(1)
	wg.Wait()
}

//Administra un hilo de la aplicacion para un id determinado.
func appManager(id string) {
	logrus.Infof("Started goroutine for id %s .", id)
	currentPage := 0
	errCount := 0
	defer wg.Done()
	for true {

		currentPage++
		//Se obtiene la summarySlice(lista de resumenes) de la pagina actual.
		summarySlice, err := searchOffertsForPage(currentPage)
		if err != nil {
			logrus.Fatal(err)
		}

		err = handlingSummaries(summarySlice, id)
		if err != nil {
			//Si la pagina a sido revisada se aumenta el contador en 1.
			if err == ErrPagRead {
				errCount++
			} else {
				logrus.Warn(err)
				logrus.Info("An error has occurred. Waiting 10 min to recover.")
				time.Sleep(10 * time.Minute)
			}
		} else {
			//Si no se lanza un error la pagina no ha sido leida, por lo tanto el
			// errCount se hace 0.
			errCount = 0
		}
		// Si se ha leido la misma pagina mas de 9 veces consecutivas, errCount se reinicia para
		// volver a enpezar el ciclo y currentPage se reinicia para empezar desde la 1ra pagina
		// para verificar si existen articulos recientes.
		if errCount >= 9 {
			currentPage = 0
			errCount = 0
			logrus.Info("All offerts are checked.Waiting for 5 hours to continue.")
			time.Sleep(5 * time.Hour)
		}
	}
}

// Maneja un resumen de ofertas para un id determinado. Si el chat al que corresponde el id
// contiene mas de 9 elementos del summarySlice retorna el error ErrPagRead.
func handlingSummaries(summarySlice OffertSummarySlice, id string) error {
	//count es la cantidad de ofertas consecutivas ya leidas por el chat actual.
	count := 0
	//Verificamos por cada elemento en el slice
	for _, val := range summarySlice.OffertSummaries {

		// Si en la de ofertas leidas por chat (para el chat actual)
		// contiene mas de de 9 elementos iguales consecutivos a los
		// elementos de la pagina se considera leida la pagina y retornamos
		// el error ErrPagRead.
		if count >= 9 {
			return ErrPagRead
		}

		//Verifica si un chat contiene el Uri de una oferta.
		ok, err := chatContainsUri(id, val.Link)
		//Si contiene el elemento pasamos a la siguiente iteracion.
		if ok {
			count++
			continue
		} else {
			// Verificamos si es un error
			if err != nil {
				return err
			}
			// Al romperse la cadena reiniciamos el contador.
			count = 0
			// Obtiene la oferta del enlace correspondiente.
			offert := getOffert(val.Link)

			// Crea el mensaje con los datos de la oferta.
			msg := fmt.Sprintf("%s:\n %s,\n %s,\n %s ", offert.Title, offert.Date, offert.Description, offert.PanelContact.Lead)

			// Envia el mensaje al chat al cual le corresponde el id.
			err = sendMsgTelegram(msg, id)
			if err != nil {
				return err
			}
			logrus.Println(fmt.Sprintf("Sender offertSummary \"Titulo: %s\" to chat %s", val.Title, id))

			// Añade la oferta actual a la lista de ofertas leidas por chat.
			err = saveUri(id, val.Link)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// searchoffertForPage obtiene la lista de resumenes para la pagina dada.
func searchOffertsForPage(pag int) (OffertSummarySlice, error) {

	var resp, err = http.Get(fmt.Sprintf("http://ofertas.cu/c/404/ofrezco/%d.html", pag))

	if err != nil {
		return OffertSummarySlice{}, err
	}

	defer resp.Body.Close()

	//summarySlice es la lista de resumenes para la pagina actual.
	var summarySlice OffertSummarySlice

	//Decodificamos el html y armamos la lista de resumenes.
	err = html.NewDecoder(resp.Body).Decode(&summarySlice)

	if err != nil {
		return OffertSummarySlice{}, err
	}
	return summarySlice, nil
}

// Guardamos el Uri de una oferta en para el chat dado segun el id.
func saveUri(id string, uri string) error {
	if err := Insert(id, uri); err != nil {
		return err
	}
	return nil
}

// Obtiene una oferta segun el URI introducido.
func getOffert(uri string) Offert {
	resp, err := http.Get(fmt.Sprintf("http://ofertas.cu%s", uri))

	if err != nil {
		logrus.Fatal(err)
	}
	defer resp.Body.Close()

	var Offrt Offert
	// Descodificamos el contenido del html en la oferta.
	err = html.NewDecoder(resp.Body).Decode(&Offrt)
	if err != nil {
		logrus.Fatal(err)
	}
	return Offrt
}

// Cargamos la lista de URI's que para un chat segun su id.
func loadUriSlice(id string) ([]string, error) {
	LinkSlice, err := Get(id)
	if err != nil && err != errBucketEmpty && err != errValEmpty {
		return nil, err
	}
	return LinkSlice, nil
}

// Envia Un mensaje (msg) a un chat segun su id (id).
func sendMsgTelegram(msg string, id string) error {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", Token), nil)
	if err != nil {
		return err
	}
	params := url.Values{}
	params.Set("chat_id", id)
	params.Set("text", msg)
	req.URL.RawQuery = params.Encode()
	_, err = http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return nil
}

// Obtiene la lista de de los id de los chat que han iniciado conversacion con el bot (getUpdates).
func getChatIds() ([]string, error) {
	resp, err := http.Get(fmt.Sprintf("https://api.telegram.org/"+
		"bot%s/getUpdates", Token))

	if err != nil {
		return nil, err
	}

	var Recive Reciver
	//Decodificamos los Updates del bot en Recive.
	err = json.NewDecoder(resp.Body).Decode(&Recive)
	if err != nil {
		return nil, err
	}
	//Lista de ids de los chats con los que el bot tiene conversacion.
	var ids []string
	for _, updt := range Recive.Result {
		exist := false
		for _, id := range ids {
			//  verifica si el id esta en la lista de ids.
			if id == updt.Message.Chat.ID {
				exist = true
			}
		}
		// Si el id no esta en la lista de ids lo añade.
		if !exist {
			ids = append(ids, updt.Message.Chat.ID)
			logrus.Infof("chat id %s", ids)
			continue
		}
		exist = false
	}
	return ids, nil
}

//
//func checkDataFile() {
//	fmt.Printf("%#v \n", ChatIds)
//	for _, id := range ChatIds {
//		_, err := os.Stat(fmt.Sprintf("%s.json", id))
//		fmt.Println(err)
//		if os.IsExist(err) {
//			err := os.Remove(fmt.Sprintf("%s.json", id))
//			if err != nil {
//				logrus.Fatal(err)
//			}
//		}
//	}
//}

// Busca recursivamente por ids mientras el bot esta corriendo.
// La duracion se va incrementando por cada ves q no encuentra resultados.
func checkForNewIds(duration uint) {
	//Pide la lista de los chatIds actuales.
	Ids, err := getChatIds()
	if err != nil {
		logrus.Fatal(err)
	}
	//Verifica si la cantidad de chatIds actuales es mayor que
	// la cantidad de chatIds registrada.
	if len(Ids) > len(ChatIds) {

		// Busca el los ids adicionales y los añade a la lista.
		for _, id := range Ids {
			exist := false
			for _, ch_id := range ChatIds {
				if ch_id == id {
					exist = true
				}
			}
			if !exist {
				//Añade el id a la lista
				ChatIds = append(ChatIds, id)
				logrus.Infof("Added %s to ChatIds list.", id)
				// Inicializa una hilo de appManage para ese id.
				go appManager(id)
				// Añade al waitGroup un una un elemento.
				wg.Add(1)
				continue
			}
			exist = false
		}
		// Duerme la funcion por el tiempo asignado (duration)
		time.Sleep(time.Duration(duration) * time.Minute)

		// Se llama recursivamente con 1 min de duracion.
		checkForNewIds(1)
		logrus.Infof("Waiting for %d minutes.", duration)

	} else {
		logrus.Infof("Waiting for %d minutes.", duration)

		// Duerme la funcion por el tiempo asignado ( duration ).
		time.Sleep(time.Duration(duration) * time.Minute)
		// Se llama recursivamente el doble de tiempo ( duration * 2 ).
		checkForNewIds(duration * 2)
	}
}

// Verifica si un chat segun su id contiene una oferta segun su URI.
func chatContainsUri(id string, uri string) (ok bool, err error) {

	// Carga la lista de URI's
	linkSlice, err := loadUriSlice(id)
	if err != nil {
		return false, err
	}
	// Verifica si la lista contiene la URI, de ser asi retorna true,
	// en caso contrario retorna false.
	if containsUri(uri, linkSlice) {
		return true, ErrLinkExist
	}

	return false, nil
}

// Verifica Si un Slice de elementos contiene un elemento dado.
func containsUri(elem interface{}, elemSlice []string) bool {
	for _, e := range elemSlice {
		if e == elem {
			return true
		}
	}
	return false
}
