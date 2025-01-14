// based on https://github.com/salrashid123/envoy_ext_proc/blob/eca3b3a89929bf8cb80879ba553798ecea1c5622/grpc_server.go

package main

import (
	"context"
	"flag"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	service_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/solo-io/go-utils/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

var (
	grpcport = flag.String("grpcport", ":18080", "grpcport")
)

type server struct{}

type healthServer struct{}

func (s *healthServer) Check(ctx context.Context, in *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	log.Printf("Handling grpc Check request: + %s", in.String())
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (s *healthServer) Watch(in *grpc_health_v1.HealthCheckRequest, srv grpc_health_v1.Health_WatchServer) error {
	return status.Error(codes.Unimplemented, "Watch is not implemented")
}

func (s *server) Process(srv service_ext_proc_v3.ExternalProcessor_ProcessServer) error {
	log.Printf("Process")
	ctx := srv.Context()
	for {
		select {
		case <-ctx.Done():
			log.Printf("context done")
			return ctx.Err()
		default:
		}

		req, err := srv.Recv()
		if err == io.EOF {
			// envoy has closed the stream. Don't return anything and close this stream entirely
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Unknown, "cannot receive stream request: %v", err)
		}

		// build response based on request type
		resp := &service_ext_proc_v3.ProcessingResponse{}
		switch v := req.Request.(type) {
		case *service_ext_proc_v3.ProcessingRequest_RequestHeaders:
			log.Printf("Got RequestHeaders")

			h := req.Request.(*service_ext_proc_v3.ProcessingRequest_RequestHeaders)
			headersResp, err := getApiVersionHeadersResponse(h.RequestHeaders, false)
			if err != nil {
				return err
			}
			resp = &service_ext_proc_v3.ProcessingResponse{
				Response: &service_ext_proc_v3.ProcessingResponse_RequestHeaders{
					RequestHeaders: headersResp,
				},
			}

		case *service_ext_proc_v3.ProcessingRequest_RequestBody:
			log.Printf("Got RequestBody (not currently handled)")

		case *service_ext_proc_v3.ProcessingRequest_RequestTrailers:
			log.Printf("Got RequestTrailers (not currently handled)")

		case *service_ext_proc_v3.ProcessingRequest_ResponseHeaders:
			log.Printf("Got ResponseHeaders")

			h := req.Request.(*service_ext_proc_v3.ProcessingRequest_ResponseHeaders)
			headersResp, err := getApiVersionHeadersResponse(h.ResponseHeaders, true)
			if err != nil {
				return err
			}
			resp = &service_ext_proc_v3.ProcessingResponse{
				Response: &service_ext_proc_v3.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: headersResp,
				},
			}

		case *service_ext_proc_v3.ProcessingRequest_ResponseBody:
			log.Printf("Got ResponseBody (not currently handled)")

		case *service_ext_proc_v3.ProcessingRequest_ResponseTrailers:
			log.Printf("Got ResponseTrailers (not currently handled)")

		default:
			log.Printf("Unknown Request type %v", v)
		}

		// At this point we believe we have created a valid response...
		// note that this is sometimes not the case
		// anyways for now just send it
		log.Printf("Sending ProcessingResponse")
		if err := srv.Send(resp); err != nil {
			log.Printf("send error %v", err)
			return err
		}

	}
}

func main() {

	flag.Parse()

	lis, err := net.Listen("tcp", *grpcport)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	sopts := []grpc.ServerOption{grpc.MaxConcurrentStreams(1000)}
	s := grpc.NewServer(sopts...)

	service_ext_proc_v3.RegisterExternalProcessorServer(s, &server{})

	grpc_health_v1.RegisterHealthServer(s, &healthServer{})

	log.Printf("Starting gRPC server on port %s", *grpcport)

	var gracefulStop = make(chan os.Signal, 1)
	signal.Notify(gracefulStop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-gracefulStop
		log.Printf("caught sig: %+v", sig)
		time.Sleep(time.Second)
		log.Printf("Graceful stop completed")
		os.Exit(0)
	}()
	err = s.Serve(lis)
	if err != nil {
		log.Fatalf("killing server with %v", err)
	}
}

func getApiVersionFromHeaders(in *service_ext_proc_v3.HttpHeaders) string {
	for _, n := range in.Headers.Headers {
		if n.Key == "api-version" {
			return string(n.RawValue)
		}
	}
	return ""
}

func checkApiVersionHeader(apiversion string) bool {
	// return true  if apiversion is   valid
	//        false if apiversion is invalid
	for _, n := range [3]string{"2024-11", "2024-12", "2025-01"} { // XXX hardcoded
		if apiversion == n {
			log.Printf("API version header is: %s.", apiversion)
			return true
		}
	}
	log.Printf("No, invalid or deprecated API version header ('%s').", apiversion)
	return false
}

func getApiVersionHeadersResponse(in *service_ext_proc_v3.HttpHeaders, responseHeader bool) (*service_ext_proc_v3.HeadersResponse, error) {
	apiVersionString := getApiVersionFromHeaders(in)

	// build the response
	resp := &service_ext_proc_v3.HeadersResponse{
		Response: &service_ext_proc_v3.CommonResponse{},
	}

	if responseHeader {
		log.Printf("Parsing response headers.")
	} else {
		log.Printf("Parsing request headers.")
	}

	// no api-version were sent, return default
	if !checkApiVersionHeader(apiVersionString) {
		log.Printf("Return default API version header.")
		var addHeaders []*core_v3.HeaderValueOption
		addHeaders = append(addHeaders, &core_v3.HeaderValueOption{
			Header: &core_v3.HeaderValue{Key: "api-version", RawValue: []byte("2024-11")}, // XXX hardcoded
		})
		if responseHeader {
			// Set "deprecation", "sunset" and "link" headers in case of response
			addHeaders = append(addHeaders, &core_v3.HeaderValueOption{
				Header: &core_v3.HeaderValue{Key: "deprecation", RawValue: []byte("@1688169599")}, // XXX hardcoded
			})
			addHeaders = append(addHeaders, &core_v3.HeaderValueOption{
				Header: &core_v3.HeaderValue{Key: "sunset", RawValue: []byte("Sun, 30 Jun 2024 23:59:59 GMT")}, // XXX hardcoded
			})
			addHeaders = append(addHeaders, &core_v3.HeaderValueOption{
				Header: &core_v3.HeaderValue{Key: "link", RawValue: []byte("<https://example.com/apis/deprecation>; rel=\"deprecation\"; type=\"text/html\"")},
			})
		}
		resp.Response.HeaderMutation = &service_ext_proc_v3.HeaderMutation{
			SetHeaders: addHeaders,
		}
	}

	return resp, nil
}
