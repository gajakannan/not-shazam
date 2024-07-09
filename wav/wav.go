package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
)

// WavHeader defines the structure of a WAV header
type WavHeader struct {
	ChunkID       [4]byte
	ChunkSize     uint32
	Format        [4]byte
	Subchunk1ID   [4]byte
	Subchunk1Size uint32
	AudioFormat   uint16
	NumChannels   uint16
	SampleRate    uint32
	BytesPerSec   uint32
	BlockAlign    uint16
	BitsPerSample uint16
	Subchunk2ID   [4]byte
	Subchunk2Size uint32
}

func writeWavHeader(f *os.File, data []byte, sampleRate int, channels int, bitsPerSample int) error {
	// Validate input
	if len(data)%channels != 0 {
		return errors.New("data size not divisible by channels")
	}

	// Calculate derived values
	subchunk1Size := uint32(16) // Assuming PCM format
	bytesPerSample := bitsPerSample / 8
	blockAlign := uint16(channels * bytesPerSample)
	subchunk2Size := uint32(len(data))

	// Build WAV header
	header := WavHeader{
		ChunkID:       [4]byte{'R', 'I', 'F', 'F'},
		ChunkSize:     uint32(36 + len(data)),
		Format:        [4]byte{'W', 'A', 'V', 'E'},
		Subchunk1ID:   [4]byte{'f', 'm', 't', ' '},
		Subchunk1Size: subchunk1Size,
		AudioFormat:   uint16(1), // PCM format
		NumChannels:   uint16(channels),
		SampleRate:    uint32(sampleRate),
		BytesPerSec:   uint32(sampleRate * channels * bytesPerSample),
		BlockAlign:    blockAlign,
		BitsPerSample: uint16(bitsPerSample),
		Subchunk2ID:   [4]byte{'d', 'a', 't', 'a'},
		Subchunk2Size: subchunk2Size,
	}

	// Write header to file
	err := binary.Write(f, binary.LittleEndian, header)
	return err
}

func WriteWavFile(filename string, data []byte, sampleRate int, channels int, bitsPerSample int) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if sampleRate <= 0 || channels <= 0 || bitsPerSample <= 0 {
		return fmt.Errorf(
			"values must be greater than zero (sampleRate: %d, channels: %d, bitsPerSample: %d)",
			sampleRate, channels, bitsPerSample,
		)
	}

	err = writeWavHeader(f, data, sampleRate, channels, bitsPerSample)
	if err != nil {
		return err
	}

	_, err = f.Write(data)
	return err
}

// WavInfo defines a struct containing information extracted from the WAV header
type WavInfo struct {
	Channels   int
	SampleRate int
	Data       []byte
	Duration   float64
}

func ReadWavInfo(filename string) (*WavInfo, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	if len(data) < 44 {
		return nil, errors.New("invalid WAV file size (too small)")
	}

	// Read header chunks
	var header WavHeader
	err = binary.Read(bytes.NewReader(data[:44]), binary.LittleEndian, &header)
	if err != nil {
		return nil, err
	}

	if string(header.ChunkID[:]) != "RIFF" || string(header.Format[:]) != "WAVE" || header.AudioFormat != 1 {
		return nil, errors.New("invalid WAV header format")
	}

	// Extract information
	info := &WavInfo{
		Channels:   int(header.NumChannels),
		SampleRate: int(header.SampleRate),
		Data:       data[44:],
	}

	// Calculate audio duration (assuming data contains PCM data)
	if header.BitsPerSample == 16 {
		info.Duration = float64(len(info.Data)) / float64(int(header.NumChannels)*2*int(header.SampleRate))
	} else {
		return nil, errors.New("unsupported bits per sample format")
	}

	return info, nil
}

// WavBytesToFloat64 converts a slice of bytes from a .wav file to a slice of float64 samples
func WavBytesToSamples(input []byte) ([]float64, error) {
	if len(input)%2 != 0 {
		return nil, errors.New("invalid input length")
	}

	numSamples := len(input) / 2
	output := make([]float64, numSamples)

	for i := 0; i < len(input); i += 2 {
		// Interpret bytes as a 16-bit signed integer (little-endian)
		sample := int16(binary.LittleEndian.Uint16(input[i : i+2]))

		// Scale the sample to the range [-1, 1]
		output[i/2] = float64(sample) / 32768.0
	}

	return output, nil
}

// FFmpegConvertWAV converts a WAV file using ffmpeg.
// It can change the sample rate and optionally convert to mono.
func FFmpegConvertWAV(inputFile, outputFile string, targetSampleRate int, toMono bool) error {
	cmdArgs := []string{
		"-i", inputFile,
		"-ar", fmt.Sprintf("%d", targetSampleRate),
		"-y",
	}

	if toMono {
		outputFile = "mono_" + outputFile
		cmdArgs = append(cmdArgs, "-ac", "1", "-c:a", "pcm_s16le")
	}

	cmdArgs = append(cmdArgs, outputFile)

	cmd := exec.Command("ffmpeg", cmdArgs...)
	return cmd.Run()
}
