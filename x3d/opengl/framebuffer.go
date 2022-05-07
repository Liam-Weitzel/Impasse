package opengl

import (
	"fmt"

	gl "github.com/go-gl/gl/v3.0/gles2"
)

func CreateFrameBuffer(width, height int32) (uint32, func(), error) {
	var fbo uint32
	gl.GenFramebuffers(1, &fbo)

	var colorBuffer uint32
	gl.GenRenderbuffers(1, &colorBuffer)
	gl.BindRenderbuffer(gl.RENDERBUFFER, colorBuffer)
	gl.RenderbufferStorage(gl.RENDERBUFFER, gl.RGBA8, width, height)
	gl.BindRenderbuffer(gl.RENDERBUFFER, 0)

	var depthBuffer uint32
	gl.GenRenderbuffers(1, &depthBuffer)
	gl.BindRenderbuffer(gl.RENDERBUFFER, depthBuffer)
	gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT32F, width, height)
	gl.BindRenderbuffer(gl.RENDERBUFFER, 0)

	// attach render buffer to the fbo as depth buffer
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, colorBuffer)
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, depthBuffer)

	free := func() {
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
		gl.DeleteRenderbuffers(1, &colorBuffer)
		gl.DeleteRenderbuffers(1, &depthBuffer)
		gl.DeleteFramebuffers(1, &fbo)
	}

	if fbs := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); fbs != gl.FRAMEBUFFER_COMPLETE {
		free()
		return 0, nil, fmt.Errorf("fbo status: %d", fbs)
	}
	return fbo, free, nil
}
