package docker

import "testing"

const exampleImage = "example/image"

func TestImageFromReference(t *testing.T) {
	testCases := []struct {
		want              Image
		name              string
		reference         string
		repositoryDigests []string
		wantOK            bool
	}{
		{
			name:      "tagged image",
			reference: exampleImage + ":1.2.3",
			repositoryDigests: []string{
				exampleImage + "@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			want: Image{
				Name:   exampleImage,
				Tag:    "1.2.3",
				Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			wantOK: true,
		},
		{
			name:      "registry with port",
			reference: "localhost:5000/example/image:1.2.3",
			repositoryDigests: []string{
				"localhost:5000/example/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			want: Image{
				Name:   "localhost:5000/example/image",
				Tag:    "1.2.3",
				Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			wantOK: true,
		},
		{
			name:      "latest without digest",
			reference: exampleImage + ":latest",
			repositoryDigests: []string{
				exampleImage + "@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			want: Image{
				Name: exampleImage,
				Tag:  "latest",
			},
			wantOK: true,
		},
		{
			name:      "tagged image with digest",
			reference: "golang:1.27.0-alpine3.24@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc",
			want: Image{
				Name:   "golang",
				Tag:    "1.27.0-alpine3.24",
				Digest: "sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc",
			},
			wantOK: true,
		},
		{
			name:      "untagged image",
			reference: exampleImage,
			wantOK:    false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, gotOK := imageFromReference(testCase.reference, testCase.repositoryDigests)

			if gotOK != testCase.wantOK {
				t.Fatalf("imageFromReference() returned ok = %t, want %t", gotOK, testCase.wantOK)
			}
			if got != testCase.want {
				t.Fatalf("imageFromReference() = %#v, want %#v", got, testCase.want)
			}
		})
	}
}
