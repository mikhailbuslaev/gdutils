package gdutils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/pawelWritesCode/qjson"
)

const (
	typeJSON        = "JSON"
	typePlainText   = "plain text"
	lastResponseKey = "LAST_HTTP_RESPONSE"
)

//bodyHeaders is entity that holds information about request body and request headers
type bodyHeaders struct {
	Body    interface{}
	Headers map[string]string
}

//ISendRequestToWithBodyAndHeaders sends HTTP request with provided body and headers.
//Argument method indices HTTP request method for example: "POST", "GET" etc.
//Argument urlTemplate should be full url path. May include template values.
//Argument bodyTemplate should be slice of bytes marshallable on bodyHeaders struct
func (s *State) ISendRequestToWithBodyAndHeaders(method, urlTemplate string, bodyTemplate *godog.DocString) error {
	input, err := s.replaceTemplatedValue(bodyTemplate.Content)
	if err != nil {
		return err
	}

	url, err := s.replaceTemplatedValue(urlTemplate)
	if err != nil {
		return err
	}

	var bodyAndHeaders bodyHeaders
	err = json.Unmarshal([]byte(input), &bodyAndHeaders)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrJson, err.Error())
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(bodyAndHeaders.Body.([]byte)))
	if err != nil {
		return fmt.Errorf("%w: %s", ErrHTTPReqRes, err.Error())
	}

	for headerName, headerValue := range bodyAndHeaders.Headers {
		req.Header.Set(headerName, headerValue)
	}

	return s.sendRequest(req)
}

//IPrepareNewRequestToAndSaveItAs prepares new request and saves it in cache under cacheKey
func (s *State) IPrepareNewRequestToAndSaveItAs(method, urlTemplate, cacheKey string) error {
	url, err := s.replaceTemplatedValue(urlTemplate)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return fmt.Errorf("%w: could not create new request, reason: %s", ErrHTTPReqRes, err.Error())
	}

	s.Cache.Save(cacheKey, req)

	return nil
}

//ISetFollowingHeadersForPreparedRequest sets provided headers for previously prepared request
func (s *State) ISetFollowingHeadersForPreparedRequest(cacheKey string, headersTemplate *godog.DocString) error {
	headers, err := s.replaceTemplatedValue(headersTemplate.Content)
	if err != nil {
		return err
	}

	var headersMap map[string]string
	if err = json.Unmarshal([]byte(headers), &headersMap); err != nil {
		return fmt.Errorf("%w: could not parse provided headers: \n\n%s\n\nerr: %s", ErrGdutils, headers, err.Error())
	}

	req, err := s.getPreparedRequest(cacheKey)
	if err != nil {
		return err
	}

	for hName, hValue := range headersMap {
		req.Header.Set(hName, hValue)
	}

	s.Cache.Save(cacheKey, req)

	return nil
}

//ISetFollowingBodyForPreparedRequest sets body for previously prepared request
//bodyTemplate may be in any format and accepts template values
func (s *State) ISetFollowingBodyForPreparedRequest(cacheKey string, bodyTemplate *godog.DocString) error {
	body, err := s.replaceTemplatedValue(bodyTemplate.Content)
	if err != nil {
		return err
	}

	req, err := s.getPreparedRequest(cacheKey)
	if err != nil {
		return err
	}

	req.Body = ioutil.NopCloser(bytes.NewReader([]byte(body)))

	s.Cache.Save(cacheKey, req)

	return nil
}

//ISendRequest sends previously prepared HTTP(s) request
func (s *State) ISendRequest(cacheKey string) error {
	req, err := s.getPreparedRequest(cacheKey)
	if err != nil {
		return err
	}

	return s.sendRequest(req)
}

//TheResponseStatusCodeShouldBe compare last response status code with given in argument.
func (s *State) TheResponseStatusCodeShouldBe(code int) error {
	lastResponse, err := s.GetLastResponse()
	if err != nil {
		return err
	}

	if lastResponse.StatusCode != code {
		return fmt.Errorf("%w: expected status code %d, but got %d", ErrHTTPReqRes, code, lastResponse.StatusCode)
	}

	return nil
}

//TheResponseBodyShouldHaveType checks whether last response body has given data type
//available data types are listed as package constants
func (s *State) TheResponseBodyShouldHaveType(dataType string) error {
	switch dataType {
	case typeJSON:
		return s.theResponseShouldBeInJSON()

	//This case checks whether last response body is not any of known types - then it assumes it is plain text
	case typePlainText:
		if err := s.theResponseShouldBeInJSON(); err != nil {
			return nil
		}

		return fmt.Errorf("%w: last response body has type %s", ErrHTTPReqRes, typeJSON)
	default:
		return fmt.Errorf("%w: unknown last response body data type, available values: %s, %s", ErrHTTPReqRes, typeJSON, typePlainText)
	}
}

//ISaveFromTheLastResponseJSONNodeAs saves from last response body JSON node under given DefaultCache key.
//expr should be valid according to qjson library
func (s *State) ISaveFromTheLastResponseJSONNodeAs(expr, cacheKey string) error {
	iVal, err := qjson.Resolve(expr, s.GetLastResponseBody())

	if err != nil {
		if s.IsDebug {
			_ = s.IPrintLastResponseBody()
		}

		return fmt.Errorf("%w, err: %s", ErrQJSON, err.Error())
	}

	s.Cache.Save(cacheKey, iVal)

	return nil
}

//IGenerateARandomIntInTheRangeToAndSaveItAs generates random integer from provided range and preserve it under given DefaultCache key
func (s *State) IGenerateARandomIntInTheRangeToAndSaveItAs(from, to int, cacheKey string) error {
	s.Cache.Save(cacheKey, randomInt(from, to))

	return nil
}

//IGenerateARandomFloatInTheRangeToAndSaveItAs generates random float from provided range and preserve it under given DefaultCache key
func (s *State) IGenerateARandomFloatInTheRangeToAndSaveItAs(from, to int, cacheKey string) error {
	randInt := randomInt(from, to)
	float01 := rand.Float64()

	strFloat := fmt.Sprintf("%.2f", float01*float64(randInt))
	floatVal, err := strconv.ParseFloat(strFloat, 64)
	if err != nil {
		return fmt.Errorf("%w: error during parsing float, err: %s", ErrGdutils, err.Error())
	}

	s.Cache.Save(cacheKey, floatVal)

	return nil
}

//IGenerateARandomStringOfLengthWithoutUnicodeCharactersAndSaveItAs generates random string of given length without unicode characters
//and preserve it under given DefaultCache key
func (s *State) IGenerateARandomStringOfLengthWithoutUnicodeCharactersAndSaveItAs(strLength int, cacheKey string) error {
	if strLength <= 0 {
		return fmt.Errorf("%w: provided string length %d can't be less than 1", ErrGdutils, strLength)
	}

	s.Cache.Save(cacheKey, s.stringWithCharset(strLength, charsetLettersOnly))

	return nil
}

//IGenerateARandomStringOfLengthWithUnicodeCharactersAndSaveItAs generates random string of given length with unicode characters
//and preserve it under given DefaultCache key
func (s *State) IGenerateARandomStringOfLengthWithUnicodeCharactersAndSaveItAs(strLength int, cacheKey string) error {
	if strLength <= 0 {
		return fmt.Errorf("%w: provided string length %d can't be less than 1", ErrGdutils, strLength)
	}

	s.Cache.Save(cacheKey, s.stringWithCharset(strLength, charsetUnicodeCharacters))

	return nil
}

//TheJSONResponseShouldHaveNode checks whether last response body contains given key
func (s *State) TheJSONResponseShouldHaveNode(expr string) error {
	_, err := qjson.Resolve(expr, s.GetLastResponseBody())
	if err != nil {
		if s.IsDebug {
			_ = s.IPrintLastResponseBody()
		}

		return fmt.Errorf("%w, node '%s', err: %s", ErrQJSON, expr, err.Error())
	}

	return nil
}

//TheJSONNodeShouldNotBe checks whether JSON node from last response body is not of provided type
//goType may be one of: nil, string, int, float, bool, map, slice
//expr should be valid according to qjson library
func (s *State) TheJSONNodeShouldNotBe(expr string, goType string) error {
	iNodeVal, err := qjson.Resolve(expr, s.GetLastResponseBody())
	if err != nil {
		return fmt.Errorf("%w, node '%s', err: %s", ErrQJSON, expr, err.Error())
	}

	vNodeVal := reflect.ValueOf(iNodeVal)
	errInvalidType := fmt.Errorf("%w: %s value is \"%s\", but expected not to be", ErrGdutils, expr, goType)
	switch goType {
	case "nil":
		if !vNodeVal.IsValid() || valueIsNil(vNodeVal) {
			return errInvalidType
		}

		return nil
	case "string":
		if vNodeVal.Kind() == reflect.String {
			return errInvalidType
		}

		return nil
	case "int":
		if vNodeVal.Kind() == reflect.Int64 || vNodeVal.Kind() == reflect.Int32 || vNodeVal.Kind() == reflect.Int16 ||
			vNodeVal.Kind() == reflect.Int8 || vNodeVal.Kind() == reflect.Int || vNodeVal.Kind() == reflect.Uint ||
			vNodeVal.Kind() == reflect.Uint8 || vNodeVal.Kind() == reflect.Uint16 || vNodeVal.Kind() == reflect.Uint32 ||
			vNodeVal.Kind() == reflect.Uint64 {
			return errInvalidType
		}

		if vNodeVal.Kind() == reflect.Float64 {
			_, frac := math.Modf(vNodeVal.Float())
			if frac == 0 {
				return errInvalidType
			}
		}

		return nil
	case "float":
		if vNodeVal.Kind() == reflect.Float64 || vNodeVal.Kind() == reflect.Float32 {
			_, frac := math.Modf(vNodeVal.Float())
			if frac == 0 {
				return nil
			}

			return errInvalidType
		}

		return nil
	case "bool":
		if vNodeVal.Kind() == reflect.Bool {
			return errInvalidType
		}

		return nil
	case "map":
		if vNodeVal.Kind() == reflect.Map {
			return errInvalidType
		}

		return nil
	case "slice":
		if vNodeVal.Kind() == reflect.Slice {
			return errInvalidType
		}

		return nil
	default:
		return fmt.Errorf("%w: %s is unknown type for this step", ErrGdutils, goType)
	}
}

//TheJSONNodeShouldBe checks whether JSON node from last response body is of provided type
//goType may be one of: nil, string, int, float, bool, map, slice
//expr should be valid according to qjson library
func (s *State) TheJSONNodeShouldBe(expr string, goType string) error {
	errInvalidType := fmt.Errorf("%w: %s value is not \"%s\", but expected to be", ErrGdutils, expr, goType)

	switch goType {
	case "nil", "string", "int", "float", "bool", "map", "slice":
		if err := s.TheJSONNodeShouldNotBe(expr, goType); err == nil {
			return errInvalidType
		}

		return nil
	default:
		return fmt.Errorf("%w: %s is unknown type for this step", ErrGdutils, goType)
	}
}

//TheJSONResponseShouldHaveNodes checks whether last request body has keys defined in string separated by comma
//nodeExpr should be valid according to qjson library expressions separated by comma (,)
func (s *State) TheJSONResponseShouldHaveNodes(nodeExprs string) error {
	keysSlice := strings.Split(nodeExprs, ",")

	errs := make([]error, 0, len(keysSlice))
	for _, key := range keysSlice {
		trimmedKey := strings.TrimSpace(key)
		_, err := qjson.Resolve(trimmedKey, s.GetLastResponseBody())

		if err != nil {
			errs = append(errs, fmt.Errorf("%w, node '%s', err: %s", ErrQJSON, trimmedKey, err.Error()))
		}
	}

	if len(errs) > 0 {
		var errString string
		for _, err := range errs {
			errString += fmt.Sprintf("%s\n", err)
		}

		if s.IsDebug {
			_ = s.IPrintLastResponseBody()
		}

		return errors.New(errString)
	}

	return nil
}

//TheJSONNodeShouldBeSliceOfLength checks whether given key is slice and has given length
//expr should be valid according to qjson library
func (s *State) TheJSONNodeShouldBeSliceOfLength(expr string, length int) error {
	iValue, err := qjson.Resolve(expr, s.GetLastResponseBody())
	if err != nil {
		if s.IsDebug {
			_ = s.IPrintLastResponseBody()
		}

		return fmt.Errorf("%w, node '%s', err: %s", ErrQJSON, expr, err.Error())
	}

	v := reflect.ValueOf(iValue)
	if v.Kind() == reflect.Slice {
		if v.Len() != length {
			return fmt.Errorf("%w: %s slice has length: %d, expected: %d", ErrGdutils, expr, v.Len(), length)
		}

		return nil
	}
	if s.IsDebug {
		_ = s.IPrintLastResponseBody()
	}

	return fmt.Errorf("%w: %s does not point at slice(array) in last HTTP(s) response JSON body", ErrGdutils, expr)
}

//TheJSONNodeShouldBeOfValue compares json node value from expression to expected by user dataValue of given by user dataType
//available data types are listed in switch section in each case directive
//expr should be valid according to qjson library
func (s *State) TheJSONNodeShouldBeOfValue(expr, dataType, dataValue string) error {
	nodeValueReplaced, err := s.replaceTemplatedValue(dataValue)
	if err != nil {
		return err
	}

	if s.IsDebug {
		fmt.Printf("Replaced value: %s\n", nodeValueReplaced)
	}

	iValue, err := qjson.Resolve(expr, s.GetLastResponseBody())
	if err != nil {
		if s.IsDebug {
			_ = s.IPrintLastResponseBody()
		}

		return fmt.Errorf("%w, node '%s', err: %s", ErrQJSON, expr, err.Error())
	}

	switch dataType {
	case "string":
		strVal, ok := iValue.(string)
		if !ok {
			if s.IsDebug {
				_ = s.IPrintLastResponseBody()
			}
			return fmt.Errorf("%w: expected %s to be %s, got %v", ErrGdutils, expr, dataType, iValue)
		}

		if strVal != nodeValueReplaced {
			if s.IsDebug {
				_ = s.IPrintLastResponseBody()
			}
			return fmt.Errorf("%w: node %s string value: %s is not equal to expected string value: %s", ErrGdutils, expr, strVal, nodeValueReplaced)
		}
	case "int":
		floatVal, ok := iValue.(float64)
		if !ok {
			if s.IsDebug {
				_ = s.IPrintLastResponseBody()
			}
			return fmt.Errorf("%w: expected %s to be %s, got %v", ErrGdutils, expr, dataType, iValue)
		}

		intVal := int(floatVal)

		intNodeValue, err := strconv.Atoi(nodeValueReplaced)

		if err != nil {
			if s.IsDebug {
				_ = s.IPrintLastResponseBody()
			}
			return fmt.Errorf("%w: replaced node %s value %s could not be converted to int", ErrGdutils, expr, nodeValueReplaced)
		}

		if intVal != intNodeValue {
			if s.IsDebug {
				_ = s.IPrintLastResponseBody()
			}
			return fmt.Errorf("%w: node %s int value: %d is not equal to expected int value: %d", ErrGdutils, expr, intVal, intNodeValue)
		}
	case "float":
		floatVal, ok := iValue.(float64)
		if !ok {
			if s.IsDebug {
				_ = s.IPrintLastResponseBody()
			}
			return fmt.Errorf("%w: expected %s to be %s, got %v", ErrGdutils, expr, dataType, iValue)
		}

		floatNodeValue, err := strconv.ParseFloat(nodeValueReplaced, 64)
		if err != nil {
			if s.IsDebug {
				_ = s.IPrintLastResponseBody()
			}
			return fmt.Errorf("%w: replaced node %s value %s could not be converted to float64", ErrGdutils, expr, nodeValueReplaced)
		}

		if floatVal != floatNodeValue {
			if s.IsDebug {
				_ = s.IPrintLastResponseBody()
			}
			return fmt.Errorf("%w: node %s float value %f is not equal to expected float value %f", ErrGdutils, expr, floatVal, floatNodeValue)
		}
	case "bool":
		boolVal, ok := iValue.(bool)
		if !ok {
			if s.IsDebug {
				_ = s.IPrintLastResponseBody()
			}
			return fmt.Errorf("%w: expected %s to be %s, got %v", ErrGdutils, expr, dataType, iValue)
		}

		boolNodeValue, err := strconv.ParseBool(nodeValueReplaced)
		if err != nil {
			if s.IsDebug {
				_ = s.IPrintLastResponseBody()
			}
			return fmt.Errorf("%w: replaced node %s value %s could not be converted to bool", ErrGdutils, expr, nodeValueReplaced)
		}

		if boolVal != boolNodeValue {
			if s.IsDebug {
				_ = s.IPrintLastResponseBody()
			}
			return fmt.Errorf("%w: node %s bool value %t is not equal to expected bool value %t", ErrGdutils, expr, boolVal, boolNodeValue)
		}
	}

	return nil
}

// TheResponseShouldHaveHeader checks whether last HTTP response has given header
func (s *State) TheResponseShouldHaveHeader(name string) error {
	defer func() {
		if s.IsDebug {
			fmt.Printf("last HTTP response headers: %+v", s.GetLastResponseHeaders())
		}
	}()

	headers := s.GetLastResponseHeaders()

	header := headers.Get(name)
	if header != "" {
		return nil
	}

	if s.IsDebug {
		fmt.Printf("last HTTP response headers: %+v", headers)
	}

	return fmt.Errorf("%w: could not find header %s in last HTTP response", ErrHTTPReqRes, name)
}

// TheResponseShouldHaveHeaderOfValue checks whether last HTTP response has given header with provided value
func (s *State) TheResponseShouldHaveHeaderOfValue(name, value string) error {
	defer func() {
		if s.IsDebug {
			fmt.Printf("last HTTP response headers: %+v", s.GetLastResponseHeaders())
		}
	}()

	headers := s.GetLastResponseHeaders()

	header := headers.Get(name)

	if header == "" && value == "" {
		return fmt.Errorf("%w: could not find header %s in last HTTP response", ErrHTTPReqRes, name)
	}

	if header == value {
		return nil
	}

	return fmt.Errorf("%w: %s header exists but, expected value: %s, is not equal to actual: %s", ErrHTTPReqRes, name, value, header)
}

//IWait waits for given timeInterval amount of time
//timeInterval should be string valid for time.ParseDuration func
func (s *State) IWait(timeInterval string) error {
	duration, err := time.ParseDuration(timeInterval)
	if err != nil {
		return fmt.Errorf("%w: something wrong with provided time interval %s, err: %s", ErrGdutils, timeInterval, err.Error())
	}

	time.Sleep(duration)
	return nil
}

//IPrintLastResponseBody prints last response from request
func (s *State) IPrintLastResponseBody() error {
	var tmp map[string]interface{}
	err := json.Unmarshal(s.GetLastResponseBody(), &tmp)

	if err != nil {
		fmt.Println(string(s.GetLastResponseBody()))
		return nil
	}

	indentedRespBody, err := json.MarshalIndent(tmp, "", "\t")

	if err != nil {
		fmt.Println(string(s.GetLastResponseBody()))
		return nil
	}

	fmt.Println(string(indentedRespBody))
	return nil
}

//IStartDebugMode starts debugging mode
func (s *State) IStartDebugMode() error {
	s.IsDebug = true

	return nil
}

//IStopDebugMode stops debugging mode
func (s *State) IStopDebugMode() error {
	s.IsDebug = false

	return nil
}
