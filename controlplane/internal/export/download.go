package export

import "context"

type Download struct {
	MaxBytes int
}

func (d Download) Method() Method {
	return Method{
		ID:      MethodDownload,
		Label:   "Download zip",
		Enabled: true,
	}
}

func (d Download) Export(ctx context.Context, req Request) (Result, error) {
	body, err := buildZip(ctx, req, d.MaxBytes)
	if err != nil {
		return Result{}, err
	}
	return Result{
		MediaType: "application/zip",
		Filename:  zipFilename(req.Project.Name),
		Body:      body,
	}, nil
}
