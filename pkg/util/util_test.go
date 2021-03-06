package util

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"syscall"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"
)

const pattern = "^[a-zA-Z0-9]+$"
const TestImagesDir = "../../tests/images"

var fileDir, _ = filepath.Abs(TestImagesDir)

var _ = Describe("Util", func() {
	It("Should match RandAlphaNum", func() {
		got := RandAlphaNum(8)
		Expect(len(got)).To(Equal(8))
		Expect(regexp.MustCompile(pattern).Match([]byte(got))).To(BeTrue())
	})

	table.DescribeTable("Find Namespace", func(inputFile, expectedResult string) {
		result := getNamespace(inputFile)
		Expect(result).To(Equal(expectedResult))
	},
		table.Entry("Valid namespace", filepath.Join(fileDir, "namespace.txt"), "test-namespace"),
		table.Entry("Invalid file", "doesnotexist", "cdi"),
	)
})

var _ = Describe("GetNameSpace", func() {
	It("Report default namespace outside container", func() {
		Expect("cdi").To(Equal(GetNamespace()))
	})
})

var _ = Describe("ParseEnv", func() {
	BeforeEach(func() {
		os.Setenv("value1", "value1")
		os.Setenv("value2", base64.StdEncoding.EncodeToString([]byte("value2")))
		os.Setenv("value3", "invalid --- *** &&&")
	})

	AfterEach(func() {
		os.Unsetenv("value1")
		os.Unsetenv("value2")
		os.Unsetenv("value3")
	})

	It("Parse environment unencoded variables", func() {
		result, err := ParseEnvVar("value1", false)
		Expect(result).To(Equal("value1"))
		Expect(err).ToNot(HaveOccurred())
		result, err = ParseEnvVar("value1", true)
		Expect(result).ToNot(Equal("value1"))
		Expect(err).To(HaveOccurred())

		result, err = ParseEnvVar("value2", false)
		Expect(result).ToNot(Equal("value2"))
		Expect(err).ToNot(HaveOccurred())
		result, err = ParseEnvVar("value2", true)
		Expect(result).To(Equal("value2"))
		Expect(err).ToNot(HaveOccurred())

		_, err = ParseEnvVar("value3", true)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("Compare quantities", func() {
	It("Should properly compare quantities", func() {
		small := resource.NewScaledQuantity(int64(1000), 0)
		big := resource.NewScaledQuantity(int64(10000), 0)
		result := MinQuantity(small, big)
		Expect(result).To(Equal(*small))
		result = MinQuantity(big, small)
		Expect(result).To(Equal(*small))
	})
})

var _ = Describe("Copy files", func() {
	var destTmp string
	var err error

	BeforeEach(func() {
		destTmp, err = ioutil.TempDir("", "dest")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err = os.RemoveAll(destTmp)
		Expect(err).NotTo(HaveOccurred())
		os.Remove("test.txt")
	})

	It("Should copy file from source to dest, with valid source and dest", func() {
		err = CopyFile(filepath.Join(TestImagesDir, "content.tar"), filepath.Join(destTmp, "target.tar"))
		Expect(err).ToNot(HaveOccurred())
		sourceMd5, err := md5sum(filepath.Join(TestImagesDir, "content.tar"))
		Expect(err).ToNot(HaveOccurred())
		targetMd5, err := md5sum(filepath.Join(destTmp, "target.tar"))
		Expect(err).ToNot(HaveOccurred())
		Expect(sourceMd5).Should(Equal(targetMd5))
	})

	It("Should not copy file from source to dest, with invalid source", func() {
		err = CopyFile(filepath.Join(TestImagesDir, "content.tar22"), filepath.Join(destTmp, "target.tar"))
		Expect(err).To(HaveOccurred())
	})

	It("Should not copy file from source to dest, with invalid target", func() {
		err = CopyFile(filepath.Join(TestImagesDir, "content.tar"), filepath.Join("/invalidpath", "target.tar"))
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("RetryBackoffSize", func() {
	var blockSize int64

	BeforeEach(func() {
		var stat syscall.Statfs_t
		err := syscall.Statfs(".", &stat)
		Expect(err).ToNot(HaveOccurred())
		blockSize = int64(stat.Bsize)
	})

	It("Should succeed", func() {
		callCount := 0
		startQuantity := resource.NewScaledQuantity(int64(250*blockSize), 0)
		err := RetryBackoffSize("", *startQuantity, func(dest string, size resource.Quantity) error {
			callCount++
			return nil
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(callCount).To(Equal(1))
	})

	It("Should succeed after 2 tries", func() {
		callCount := 0
		startQuantity := resource.NewScaledQuantity(int64(250*blockSize), 0)
		err := RetryBackoffSize("", *startQuantity, func(dest string, size resource.Quantity) error {
			callCount++
			if resource.NewScaledQuantity(int64(200*blockSize), 0).Cmp(size) == 0 {
				return nil
			}
			return fmt.Errorf("I am failing two tries, help me, %+v", size)
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(callCount).To(Equal(3))
	})

	It("Should fail after 10 tries", func() {
		callCount := 0
		startQuantity := resource.NewScaledQuantity(int64(250*blockSize), 0)
		err := RetryBackoffSize("", *startQuantity, func(dest string, size resource.Quantity) error {
			callCount++
			return fmt.Errorf("I am failing, help me")
		})
		Expect(err).To(HaveOccurred())
		Expect(callCount).To(Equal(11))
	})

	It("Should fail with invalid dest", func() {
		callCount := 0
		startQuantity := resource.NewScaledQuantity(int64(250*blockSize), 0)
		err := RetryBackoffSize("/invalid/invalid", *startQuantity, func(dest string, size resource.Quantity) error {
			callCount++
			return fmt.Errorf("I should never get called")
		})
		Expect(err).To(HaveOccurred())
		Expect(callCount).To(Equal(0))
	})
})

func md5sum(filePath string) (string, error) {
	var returnMD5String string

	file, err := os.Open(filePath)
	if err != nil {
		return returnMD5String, err
	}
	defer file.Close()

	hash := md5.New()

	if _, err := io.Copy(hash, file); err != nil {
		return returnMD5String, err
	}

	hashInBytes := hash.Sum(nil)[:16]
	returnMD5String = hex.EncodeToString(hashInBytes)

	return returnMD5String, nil
}
