#version 300 es

#ifdef GL_ES
precision highp float;
#endif

in vec2 texCoord;
in vec3 normal;
in vec3 lightVec;
in float fogDepth;

out vec4 fragColor;

uniform sampler2D texSampler;

uniform vec3 ambientCol; // The light and object's combined ambient color
uniform vec3 diffuseCol;  // The light and object's combined diffuse color
uniform bool useTexture;

const float invRadiusSq = 0.00001;
const float fogNear = 0.0;
const float fogFar = 1500.0;
const vec4 fogColor = vec4(0.0, 0.0, 0.0, 1.0);

const vec4 white = vec4(0.7, 0.7, 0.7, 1.0);

void main() {

    vec4 col;

    if (useTexture) {
        col = texture(texSampler, texCoord);
    } else {
        col = white;
    }
    // base color from diffuse texture

    // ambient lighting
    vec3 ambient = vec3(ambientCol * col.xyz);

    // Calculate the light attenuation and direction.
    float distSq = dot(lightVec, lightVec);
    float attenuation = clamp(1.0 - invRadiusSq * sqrt(distSq), 0.0, 1.0);
    attenuation *= attenuation;
    vec3 lightDir = lightVec * inversesqrt(distSq);

    // Diffuse lighting
    vec3 diffuse = max(dot(lightDir, normal), 0.0) * diffuseCol * col.xyz;

    vec3 finalCol = (ambient + diffuse)*attenuation;

    vec4 shaded = vec4(finalCol, col.w);

    float fogAmount = smoothstep(fogNear, fogFar, fogDepth);

    fragColor = mix(shaded, fogColor, fogAmount);
}
